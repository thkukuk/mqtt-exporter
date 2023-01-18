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
	"regexp"
	"strconv"

	"github.com/eclipse/paho.mqtt.golang"
)

const (
	metricPerTopicRegexGroup = "metricname"
)

type InfluxDBMapping struct {
	MqttName           string                    `yaml:"mqtt_name"`
	Name               string                    `yaml:"name,omitempty"`
	Unit               string                    `yaml:"unit,omitempty"`
	Type               string                    `yaml:"type,omitempty"`
	ConstantTags       map[string]string         `yaml:"const_tags"`
	StringValueMapping *StringValueMappingConfig `yaml:"string_value_mapping,omitempty"`
}

type StringValueMappingConfig struct {
        // ErrorValue is used when no mapping is found in Map
        ErrorValue int            `yaml:"error_value"`
        Map        map[string]int `yaml:"map"`
}

var (
	metricPerTopicRegex *regexp.Regexp
)

// metricPerTopicValue returns the metric name.
func metricPerTopicValue(topic string) string {
	if metricPerTopicRegex == nil {
		return ""
	}

        match := metricPerTopicRegex.FindStringSubmatch(topic)
        values := make(map[string]string)
        for i, name := range metricPerTopicRegex.SubexpNames() {
                if len(match) > i && name != "" {
			values[name] = match[i]
                }
        }
        return values[metricPerTopicRegexGroup]
}

func msg2dbentry(dbmapping []InfluxDBMapping, msg mqtt.Message) (string, map[string]string, map[string]interface{}, error) {
	deviceID := deviceIDValue(msg.Topic())
	if len(deviceID) == 0 {
		return "", nil, nil, nil // No deviceID, so ignore this message
	}

	metricName := metricPerTopicValue(msg.Topic())
	if len(metricName) == 0 {
		return "", nil, nil, nil // not for us
	}

	if Verbose {
		logger.Printf("- Device ID: %q, Metric name: %q",
			deviceID, metricName)
	}

	var field map[string]interface{}
	var err error

	for i := range dbmapping {
		if metricName != dbmapping[i].MqttName {
			continue
		}
		if len(dbmapping[i].Name) == 0 {
			dbmapping[i].Name = dbmapping[i].MqttName
		}
		if dbmapping[i].StringValueMapping != nil {
			v := dbmapping[i].StringValueMapping.ErrorValue
			for k := range dbmapping[i].StringValueMapping.Map {
				if string(msg.Payload()[:]) == k {
					v = dbmapping[i].StringValueMapping.Map[k]
				}
			}
			field = map[string]interface{}{dbmapping[i].Name: v}
		} else if dbmapping[i].Type == "float" {
			var f float64
			if f, err = strconv.ParseFloat(string(msg.Payload()[:]), 64); err != nil {
				logerr.Printf("Cannot convert '%s' to float64: %v", msg.Payload(), err)
			} else {
				field = map[string]interface{}{Config.DBMapping[i].Name: f}
			}
		} else if dbmapping[i].Type == "int" {
			var f int64
			if f, err = strconv.ParseInt(string(msg.Payload()[:]), 10, 0); err != nil {
				logerr.Printf("Cannot convert '%s' to int64: %v", msg.Payload(), err)
			} else {
				field = map[string]interface{}{dbmapping[i].Name: f}
			}
		} else  if dbmapping[i].Type == "string" {
			field = map[string]interface{}{dbmapping[i].Name: msg.Payload()}
		}

		tags := make(map[string]string)
		for v, k := range dbmapping[i].ConstantTags {
			tags[k] = v
		}
		if len(dbmapping[i].Unit) > 0 {
			tags["unit"] = dbmapping[i].Unit
		}

		return deviceID, tags, field, nil
	}

	return "", nil, nil, nil
}
