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
	"log"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/influxdata/influxdb-client-go/v2"

	"github.com/thkukuk/mqtt-exporter/pkg/influxdb"
)

const (
	deviceIDRegexGroup = "deviceid"
	defMQTTPort = "1883"
	defInfluxDBdatabase = "my-bucket"
)

type ConfigType struct {
        MQTT            *MQTTConfig              `yaml:"mqtt,omitempty"`
	InfluxDB        *influxdb.InfluxDBConfig `yaml:"influxdb,omitempty"`
	DBMapping       []InfluxDBMapping        `yaml:"db_mapping"`
}

type MQTTConfig struct {
        Broker                 string `yaml:"broker"`
	Port                   string `yaml:"port"`
        TopicPath              string `yaml:"topic_path"`
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
	logger  = log.New(os.Stdout, "", log.LstdFlags)
	logerr  = log.New(os.Stderr, "", log.LstdFlags)
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
		logger.Printf("Received message: topic: %s - %s\n", msg.Topic(), msg.Payload())
	}

	// XXX error handling
	id, unit, field, _ := msg2dbentry(Config.DBMapping, msg)

	if len(id) > 0 {
		if Verbose {
			logger.Printf("- WriteEntry(%s, %v, %v)", id, unit, field)
		}
		_ = influxdb.WriteEntry(db, *Config.InfluxDB, id, unit, field)
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	logger.Println("Connection established")

	// Establish the subscription - doing this here means that it
	// will happen every time a connection is established
	// (useful if opts.CleanSession is TRUE or the broker does not
	// reliably store session data)
	token := client.Subscribe(Config.MQTT.TopicPath,
		Config.MQTT.QoS, msgHandler)

	// the connection handler is called in a goroutine so blocking
	// here would hot cause an issue. However as blocking in other
	// handlers does cause problems its best to just assume we should
	// not block
	go func() {
		//_ = token.Wait()
		<-token.Done()

		if token.Error() != nil {
			logerr.Printf("ERROR subscribing: %s", token.Error())
		} else {
			if !Quiet {
				logger.Printf("Subscribed to topic: %s",
					Config.MQTT.TopicPath)
			}
		}
	}()
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	logerr.Printf("Connection lost: %v", err)
}

func RunServer() {
	if !Quiet {
		logger.Printf("MQTT Exporter (mqtt-exporter) %s is starting...\n", Version)
	}

	var mqtt_client mqtt.Client

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	signal.Notify(quit, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Print("Terminated via Signal. Shutting down...")
		if mqtt_client.IsConnectionOpen() {
			mqtt_client.Disconnect(250)
		}
		os.Exit(0)
	}()

	var err error
	deviceIDRegex, err = regexp.Compile(Config.MQTT.DeviceIDPattern)
	if err != nil {
		logerr.Fatal(err)
	}
	var validRegex bool
        for _, name := range deviceIDRegex.SubexpNames() {
                if name == deviceIDRegexGroup {
                        validRegex = true
                }
        }
        if !validRegex {
		logerr.Fatalf("device id regex %q does not contain required regex group %q",
			Config.MQTT.DeviceIDPattern, deviceIDRegexGroup)
        }

	if len(Config.MQTT.MetricPerTopicPattern) > 0 {
		metricPerTopicRegex, err = regexp.Compile(Config.MQTT.MetricPerTopicPattern)
		if err != nil {
			logerr.Fatalf("Error compiling metric_per_topic_regex: %v", err)
		}
		for _, name := range metricPerTopicRegex.SubexpNames() {
			if name == metricPerTopicRegexGroup {
				validRegex = true
			}
		}
		if !validRegex {
			logerr.Fatalf("metric_per_topic_regex %q does not contain required regex group %q",
				Config.MQTT.MetricPerTopicPattern, metricPerTopicRegexGroup)
		}
	}

	if Config.InfluxDB != nil {
		if len(Config.InfluxDB.Database) == 0 {
			Config.InfluxDB.Database = defInfluxDBdatabase
		}
		db = influxdb.ConnectInfluxDB(Config.InfluxDB)
	} else {
		logger.Fatal("No InfluxDB server specified!")
	}

	opts := mqtt.NewClientOptions()

	if len(Config.MQTT.Port) == 0 {
		Config.MQTT.Port = defMQTTPort
	}

	brokerUrl := fmt.Sprintf("tcp://%s:%s",
		Config.MQTT.Broker, Config.MQTT.Port)
	if !Quiet {
		logger.Printf("Broker: %s", brokerUrl)
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
			logger.Printf("Could not connect to mqtt broker, sleep 10 second: %v", token.Error())
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
                        logerr.Printf("Error while processing message: %v", err)
                }
        }
}
