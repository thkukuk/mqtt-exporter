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

package main

import (
	"fmt"
        "io/ioutil"
        "log"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
	"github.com/thkukuk/mqtt-exporter/pkg/mqtt-exporter"
)

var (
	configFile = "config.yaml"
)

func read_yaml_config(conffile string) (mqttExporter.ConfigType, error) {

        var config mqttExporter.ConfigType

        file, err := ioutil.ReadFile(conffile)
        if err != nil {
                return config, fmt.Errorf("Cannot read %q: %v", conffile, err)
        }
        err = yaml.Unmarshal(file, &config)
        if err != nil {
                return config, fmt.Errorf("Unmarshal error: %v", err)
        }

        return config, nil
}


func main() {
// mqttExporterCmd represents the mqtt-exporter command
	mqttExporterCmd := &cobra.Command{
		Use:   "mqtt-exporter",
		Short: "Starts a MQTT Exporter",
		Long: `Starts a MQTT Exporter.
This daemon listens to MQTT topics and forwards them to InfluxDB.
`,
		Run: runMqttExporterCmd,
		Args:  cobra.ExactArgs(0),
	}

        mqttExporterCmd.Version = mqttExporter.Version

	mqttExporterCmd.Flags().StringVarP(&configFile, "config", "c", configFile, "configuration file")

	mqttExporterCmd.Flags().BoolVarP(&mqttExporter.Quiet, "quiet", "q", mqttExporter.Quiet, "don't print any informative messages")
	mqttExporterCmd.Flags().BoolVarP(&mqttExporter.Verbose, "verbose", "v", mqttExporter.Verbose, "become really verbose in printing messages")

	if err := mqttExporterCmd.Execute(); err != nil {
                os.Exit(1)
        }
}

func runMqttExporterCmd(cmd *cobra.Command, args []string) {
	var err error

	if !mqttExporter.Quiet {
		log.Printf("Read yaml config %q\n", configFile)
	}
	mqttExporter.Config, err = read_yaml_config(configFile)
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

        mqtt_user := os.Getenv("MQTT_USER")
        if mqtt_user != "" {
                mqttExporter.Config.MQTT.User = mqtt_user
        }

	mqtt_password := os.Getenv("MQTT_PASSWORD")
        if mqtt_password != "" {
                mqttExporter.Config.MQTT.Password = mqtt_password
        }

	mqttExporter.RunServer()
}
