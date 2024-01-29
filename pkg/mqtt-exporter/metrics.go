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
	"regexp"
	"strconv"
	"strings"

	log "github.com/thkukuk/mqtt-exporter/pkg/logger"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/thedevsaddam/gojsonq/v2"
)

const (
	metricPerTopicRegexGroup = "metricname"
)

type MetricsType struct {
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

func msg2dbentry(metrics []MetricsType, msg mqtt.Message) (string, map[string]string, map[string]interface{}, error) {
	deviceID := deviceIDValue(msg.Topic())
	if len(deviceID) == 0 {
		return "", nil, nil, nil // No deviceID, so ignore this message
	}

	metricName := metricPerTopicValue(msg.Topic())
	if len(metricName) == 0 {
		return "", nil, nil, nil // not for us
	}

	if Verbose {
		log.Debugf("- Device ID: %q, Metric name: %q",
			deviceID, metricName)
	}

	var tags = make(map[string]string)
	var field = make(map[string]interface{})
	var err error

	found := false

	for i := range metrics {
		mqttName := metrics[i].MqttName
		isJson := false

		if strings.Contains(mqttName, ".") {
			isJson = true
			mqttName = mqttName[:strings.IndexByte(mqttName, '.')]
		}

		if metricName != mqttName {
			continue
		}
		if len(metrics[i].Name) == 0 {
			metrics[i].Name = metrics[i].MqttName
		}

		payload := string(msg.Payload())
		if isJson {
			// gojsonq.Find is of form "a.b.c.d", where "a" is
			// the mqttName. So remove it, it's not part of the
			// json struct
			jsonFind := metrics[i].MqttName[(strings.IndexByte(metrics[i].MqttName, '.')+1):]
			entry := gojsonq.New().FromString(payload).Find(jsonFind)
			if entry == nil {
				if Verbose {
					log.Warnf("WARNING: %q not found in '%s'!", jsonFind, payload)
				}
				continue
			}
			payload = fmt.Sprintf("%v", entry)
		}

		if metrics[i].StringValueMapping != nil {
			v := metrics[i].StringValueMapping.ErrorValue
			for k := range metrics[i].StringValueMapping.Map {
				if payload[:] == k {
					v = metrics[i].StringValueMapping.Map[k]
				}
			}
			field[metrics[i].Name] = v
		} else if metrics[i].Type == "float" {
			var f float64
			if f, err = strconv.ParseFloat(payload[:], 64); err != nil {
				log.Errorf("%s: cannot convert '%s' to float64: %v",
					deviceID, payload, err)
			} else {
				field[Config.Metrics[i].Name] = f
			}
		} else if metrics[i].Type == "int" || metrics[i].Type == "integer" {
			var f int64
			if f, err = strconv.ParseInt(payload[:], 10, 0); err != nil {
				log.Errorf("%s: cannot convert '%s' to int64: %v",
					deviceID, payload, err)
			} else {
				field[metrics[i].Name] = f
			}
		} else  if metrics[i].Type == "string" {
			field[metrics[i].Name] = payload
		}

		for v, k := range metrics[i].ConstantTags {
			tags[k] = v
		}

		// XXX json structs and unit -> last one wins...
		if len(metrics[i].Unit) > 0 {
			tags["unit"] = metrics[i].Unit
		}

		found = true

		if !isJson {
			// if this is not a json struct, there cannot
			// be more entries, so safe time and return
			return deviceID, tags, field, nil
		}
	}

	if found {
		return deviceID, tags, field, nil
	} else {
		return "", nil, nil, nil
	}
}
