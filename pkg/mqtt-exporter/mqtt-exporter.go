// Copyright 2023 Thorsten Kukuk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mqttExporter

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/influxdata/influxdb-client-go/v2"
)

const (
	deviceIDRegexGroup = "deviceid"
	defMQTTPort = "1883"
	defMQTTSPort = "8883"
	defMQTTProtocol = "mqtt"
	defMQTTSProtocol = "mqtts"
	defInfluxDBdatabase = "my-bucket"
)

type ConfigType struct {
        MQTT            *MQTTConfig     `yaml:"mqtt"`
	InfluxDB        *InfluxDBConfig `yaml:"influxdb,omitempty"`
	Metrics         []MetricsType   `yaml:"metrics"`
}

type MQTTConfig struct {
        Broker                 string `yaml:"broker"`
	Port                   string `yaml:"port"`
	Protocol               string `yaml:"protocol"`
        TopicPaths             []string `yaml:"topic_paths"`
        DeviceIDPattern        string `yaml:"device_id_regex"`
        User                   string `yaml:"user"`
        Password               string `yaml:"password"`
        ClientID               string `yaml:"client_id"`
        QoS                    byte   `yaml:"qos"`
	MetricPerTopicPattern  string `yaml:"metric_per_topic_regex"`
}

var (
	Version = "unreleased"
	Quiet   = false
	Verbose = false
	Config ConfigType
	db influxdb2.Client
	deviceIDRegex *regexp.Regexp
)

func createMQTTClientID() string {
	host, err := os.Hostname()
        if err != nil {
		// XXX we should implement correct error handling
                panic(fmt.Sprintf("failed to get hostname: %v", err))
        }
        pid := os.Getpid()
        return fmt.Sprintf("%s-%d", host, pid)
}

// DeviceIDValue returns the device ID.
func deviceIDValue(topic string) string {
        match := deviceIDRegex.FindStringSubmatch(topic)
        values := make(map[string]string)
        for i, name := range deviceIDRegex.SubexpNames() {
                if len(match) > i && name != "" {
			values[name] = match[i]
                }
        }
        return values[deviceIDRegexGroup]
}

func msgHandler(client mqtt.Client, msg mqtt.Message) {
	if Verbose {
		log.Debugf("Received message: topic: %s - %s\n", msg.Topic(), msg.Payload())
	}

	// XXX error handling
	id, unit, field, _ := msg2dbentry(Config.Metrics, msg)

	if len(id) > 0 {
		if Verbose {
			log.Debugf("- WriteEntry(%s, %v, %v)", id, unit, field)
		}
		_ = WriteEntry(db, *Config.InfluxDB, id, unit, field)
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Info("Connection to MQTT Broker established")

	// Establish the subscription - doing this here means that it
	// will happen every time a connection is established

	// the connection handler is called in a goroutine so blocking
	// here would hot cause an issue. However as blocking in other
	// handlers does cause problems its best to just assume we should
	// not block
	for i := range Config.MQTT.TopicPaths {
		topic := Config.MQTT.TopicPaths[i]
		token := client.Subscribe(topic, Config.MQTT.QoS, msgHandler)

		go func() {
			//_ = token.Wait()
			<-token.Done()

			if token.Error() != nil {
				log.Errorf("Error subscribing: %s", token.Error())
			} else {
				if !Quiet {
					log.Infof("Subscribed to topic: %s", topic)
				}
			}
		}()
	}
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Errorf("Connection to MQTT Broker lost: %v", err)
}

func RunServer() {
	if !Quiet {
		log.Infof("MQTT Exporter (mqtt-exporter) %s is starting...\n", Version)
	}

	var mqtt_client mqtt.Client

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	signal.Notify(quit, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info("Terminated via Signal. Shutting down...")
		if mqtt_client != nil && mqtt_client.IsConnectionOpen() {
			mqtt_client.Disconnect(250)
		}
		os.Exit(0)
	}()

	var err error
	deviceIDRegex, err = regexp.Compile(Config.MQTT.DeviceIDPattern)
	if err != nil {
		log.Fatal(err)
	}
	var validRegex bool
        for _, name := range deviceIDRegex.SubexpNames() {
                if name == deviceIDRegexGroup {
                        validRegex = true
                }
        }
        if !validRegex {
		log.Fatalf("device id regex %q does not contain required regex group %q",
			Config.MQTT.DeviceIDPattern, deviceIDRegexGroup)
        }

	if len(Config.MQTT.MetricPerTopicPattern) > 0 {
		metricPerTopicRegex, err = regexp.Compile(Config.MQTT.MetricPerTopicPattern)
		if err != nil {
			log.Fatalf("Error compiling metric_per_topic_regex: %v", err)
		}
		for _, name := range metricPerTopicRegex.SubexpNames() {
			if name == metricPerTopicRegexGroup {
				validRegex = true
			}
		}
		if !validRegex {
			log.Fatalf("metric_per_topic_regex %q does not contain required regex group %q",
				Config.MQTT.MetricPerTopicPattern, metricPerTopicRegexGroup)
		}
	}

	if Config.InfluxDB != nil {
		if len(Config.InfluxDB.Database) == 0 {
			Config.InfluxDB.Database = defInfluxDBdatabase
		}
		db, err = ConnectInfluxDB(Config.InfluxDB)
		if err != nil {
			log.Fatalf("Cannot connect to InfluxDB: %v", err)
		}
	} else {
		log.Fatal("No InfluxDB server specified!")
	}

	opts := mqtt.NewClientOptions()

	if len(Config.MQTT.Protocol) == 0 {
		if Config.MQTT.Port == defMQTTSPort {
			Config.MQTT.Protocol = defMQTTSProtocol
		} else {
			Config.MQTT.Protocol = defMQTTProtocol
		}
	}

	if len(Config.MQTT.Port) == 0 {
		if Config.MQTT.Protocol == defMQTTSProtocol {
			Config.MQTT.Port = defMQTTSPort
		} else {
			Config.MQTT.Port = defMQTTPort
		}
	}

	brokerUrl := fmt.Sprintf("%s://%s:%s",
		Config.MQTT.Protocol, Config.MQTT.Broker,
		Config.MQTT.Port)
	if !Quiet {
		log.Printf("Broker: %s", brokerUrl)
	}

	opts.AddBroker(brokerUrl)
	opts.SetAutoReconnect(true)
	if len(Config.MQTT.ClientID) > 0 {
		opts.SetClientID(Config.MQTT.ClientID)
	} else {
		opts.SetClientID(createMQTTClientID())
	}
	if len(Config.MQTT.User) > 0 {
		opts.SetUsername(Config.MQTT.User)
	}
	if len(Config.MQTT.Password) > 0 {
		opts.SetPassword(Config.MQTT.Password)
	}
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

        errorChan := make(chan error, 1)

        for {
		mqtt_client = mqtt.NewClient(opts)
		if token := mqtt_client.Connect(); token.Wait() && token.Error() != nil {
			log.Warnf("Could not connect to mqtt broker, sleep 10 second: %v", token.Error())
			time.Sleep(10 * time.Second)
		} else {
                        break
                }
        }

	// loop forever and print error messages if they arrive
	// app is quit with above signal handler "quit".
	for {
                select {
                case err := <-errorChan:
                        log.Errorf("Error while processing message: %v", err)
                }
        }
}
