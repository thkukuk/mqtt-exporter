# mqtt-exporter
**MQTT Exporter - listens to MQTT topics and forwards them to InfluxDB**


This exporter listens to MQTT topics and stores them in an InfluxDB database. It takes the message from the topics (which can be a single value or a JSON struct) and translates it between the MQTT represenation and the format needed for database storage. How this translation is done is specified in the configuration file for mqtt-exporter, as you normally cannot adjust the messages IoT devices send. The IoT devices will push their metrics via MQTT to an MQTT Broker and this exporter subscribes to the broker and processes the rrecived messages.

```plaintext
 IoT Sensors -> publish -> MQTT Broker <- subcribed <- MQTT-Exporter -> stores -> InfluxDB
 ```

I wrote this exporter initial to export the power metering values of my Shelly Plug S and Shelly Plus H&T devices to an InfluxDB and make them visible in Grafana.
There is a blog from me which explains how this works in more detail with many example: [Export MQTT Topics to InfluxDB](https://www.thkukuk.de/blog/mqtt-exporter/)

## Assumptions about Messages and Topics

MQTT exporter can subscribe to several topics, but there is only one handler for this topics and only one Regex for the device ID and netric name. If separate regex per topic are required, several instances of MQTT broker needs to be run.

It is possible to have devices which publish every metric in an own topic, as JSON struct per topic or a mix of both.
For Shelly Plug S and Shelly Plus H&T this MQTT messages could look like:

```plaintext
shellies/shelly-plug-s1/relay/0/power 20.58
shellies/shelly-plug-s1/relay/0/energy 94736
shellies/shelly-plug-s1/relay/0 on
shellies/shelly-plug-s1/temperature 22.28
shellies/shelly-plug-s1/temperature_f 72.10
shellies/shelly-plug-s1/overtemperature 0
shelly-ht/shelly-plus-ht-01/events/rpc {"src":"shellyplusht-08b61fce63c4","dst":"shelly-ht/shelly-plus-ht-01/events","method":"NotifyFullStatus","params":{"ts":1674473304.12,"ble":{},"cloud":{"connected":false},"devicepower:0":{"id": 0,"battery":{"V":6.17, "percent":100},"external":{"present":false}},"ht_ui":{},"humidity:0":{"id": 0,"rh":54.3},"mqtt":{"connected":true},"sys":{"mac":"08B61FCE63C4","restart_required":false,"time":null,"unixtime":null,"uptime":1,"ram_size":235504,"ram_free":165340,"fs_size":458752,"fs_free":131072,"cfg_rev":16,"kvs_rev":0,"webhook_rev":0,"available_updates":{},"wakeup_reason":{"boot":"deepsleep_wake","cause":"periodic"},"wakeup_period":7200},"temperature:0":{"id": 0,"tC":19.9, "tF":67.8},"wifi":{"sta_ip":"172.17.0.80","status":"got ip","ssid":"my-wifi","rssi":-70},"ws":{"connected":false}}}
```

The device ID, which is used for the `measurement` field when writing the data into the database, is gotten by a regular expression (see `device_id_regex` in the configuration file) from the MQTT topic. This allows an arbitrary place of the device ID in the mqtt topic. For example the tasmota firmware pushes the telemetry data to the topic `tele/<deviceid>/SENSOR`, the Shelly Plug S uses `shellies/<deviceid>/SENSOR` while the Shelly Plus H&T uses `<deviceid>/events/rpc`.

The name for the metric (`metricname`) is extracted from the topic by the regular expression `metric_per_topic_regex`.

With my config file for Shelly Plug devices, the above MQTT messages would be converted to the following database points:

```
measurement: shelly-plug-s1, tags: {"unit": "Watt"}, field: {"power": 20.58}
measurement: shelly-plug-s1, tags: ["unit": "Watt/Minute"}, field: {"energy": 94736}
measurement: shelly-plug-s1, tags: {"unit": "C"}, field: {"temperature": 22.28}
measurement: shelly-plug-s1, tags: {}, field: {"switch": 2}
measurement: shelly-plus-ht-01, tags: {}, field: {"battery_voltage": 100 "humidity": 52.9 "ip_address":XX.XX.XX.XX "temperature":20]
```

The measurement is the `device_id_regex` from the MQTT topicy, the field key is the `metricname`.

The topic path can contain multiple wildcards. MQTT has kind of two wildcards:

* `+`: Single level of hierarchy in the topic path
* `#`: Many levels of hierarchy in the topic path

The [MQTT man page](https://mosquitto.org/man/mqtt-7.html) explains this in depth in the "Topics/Subscriptions" section.

The example `topic_path: devices/+/sensors/#` will match:

```plaintext
devices/home/sensors/foo/bar
devices/workshop/sensors/temperature
```

## Container

### Public Container Image

To run the public available image:

```bash
podman run --rm -v <path>/config.yaml:/config.yaml registry.opensuse.org/home/kukuk/containerfile/mqtt-exporter
```

You can replace `podman` with `docker` without any further changes.

### Build locally

To build the container image with the `mqtt-exporter` binary included run:

```bash
sudo podman build --rm --no-cache --build-arg VERSION=$(cat VERSION) --build-arg BUILDTIME=$(date +%Y-%m-%dT%TZ) -t mqtt-exporter .
```

You can of cource replace `podman` with `docker`, no other arguments needs to be adjusted.

## Configuration

The mqtt-exporter will be configured via command line and configuration file.

### Commandline

Available options are:
```plaintext
Usage:
  mqtt-exporter [flags]

Flags:
  -c, --config string   configuration file (default "config.yaml")
  -h, --help            help for mqtt-exporter
  -q, --quiet           don't print any informative messages
  -v, --verbose         become really verbose in printing messages
      --version         version for mqtt-exporter
```

### Configuration File

By default `mqtt-exporter` looks for the file `config.yaml` in the local directory. This can be overriden with the `--config` option.

Here is my configuration file, which I use for my Shelly Plug S.

```yaml
# Optional, if set, /healthz liveness probe and /readyz readiness
# probes will be provided
#health_check: ":8080"
mqtt:
  # Required: The MQTT broker to connect to
  broker: <mqtt broker IP>
  # Optional: Port of the MQTT broker
  # 8883 is the default MQTT port using TLS, 1883 will not use TLS
  # port: 1883
  # Optional: Protocol to use
  # 'mqtts' is using TLS, 'mqtt' will not
  # protocol: mqtt
  # Optional: Username and Password for authenticating with the MQTT Server
  #user: <username>
  #password: <password>
  # Optional: Used to specify ClientID. The default is <hostname>-<pid>
  # client_id: somedevice
  # The Topic paths to subscribe to. Be aware that you have to specify the
  # wildcard. MQTT Exporter can subscribe to several topics, but all of them
  # need to match the device_id_regex and metric_per_topic_regex. There are
  # not different regex per topic. If this is required, an own MQTT Exporter
  # instance is needed.
  topic_paths:
   - shellies/#
   - shelly-ht/#
  # Optional: Regular expression to extract the device ID from the topic
  # path. The default regular expression, assumes that the last "element"
  # of the topic_path is the device id. The regular expression must contain
  # a named capture group with the name deviceid. For example the
  # expression for tasamota based sensors is "tele/(?P<deviceid>.*)/.*".
  # The default is:
  # device_id_regex: "(.*/)?(?P<deviceid>.*)"
  device_id_regex: ".*?/(?P<deviceid>.*?)/.*"
  # Optional: This regex is used to extract the metric name from the
  # topic. Must contain a named group for `metricname`.
  metric_per_topic_regex: ".*/(?P<metricname>.*)"
  # The MQTT QoS level
  qos: 0
influxdb:
  # machine on which influxdb runs on port 8086:
  server: influxdb.example.com
  # should https be used?
  # tls: true|false
  # Database or bucket or however it will be called in InfluxDB v3...
  database: shellies
  # Optional for InfluxDB v1.x, required for InfluxDB v2.x.
  organization: my-org
  # If a token is required, you can specify it here (but be careful that you
  # don't commit it a public git repo or something similar! Or you can use
  # an environment variable 'INFLUXDB_TOKEN'
  # For InfluxDB v1 this is 'username:password', for InfluxDB v2 the token
  # token: <token>
metrics:
  # The first metrics are for the Shelly Plug S
  - mqtt_name: temperature
    name: temperature
    unit: C
    type: float
  - mqtt_name: power
    name: power
    unit: Watt
    type: float
  - mqtt_name: energy
    name: energy
    unit: Watt/Minute
    type: int
  - mqtt_name: 0
    name: switch
    # Optional: enables mapping between string values to metric values.
    # type is ignored if string_value_mapping is specified
    string_value_mapping:
      map:
        off: 0
        low: 1
        on: 2
      error_value: -1
  - mqtt_name: info.update.has_update
    name: firmware_update
    unit: Boolean
    string_value_mapping:
      map:
        true: 1
        false: 0
      error_value: -1
  - mqtt_name: info.update.old_version
    name: current_firmware_version
    unit: Version
    type: string
  - mqtt_name: info.wifi_sta.ip
    name: ipaddress
    type: string
  # the following metrics are for the Shelly Plus H&T
  - mqtt_name: rpc.params.temperature:0.tC
    name: temperature
    type: float
  - mqtt_name: rpc.params.humidity:0.rh
    name: humidity
    type: float
  - mqtt_name: rpc.params.devicepower:0.battery.V
    name: battery_voltage
    type: float
  - mqtt_name: rpc.params.devicepower:0.battery.percent
    name: battery
    type: float
  - mqtt_name: rpc.params.wifi.sta_ip
    name: ip_address
    type: string

```

### Explanation

The metrics section defines, for which MQTT topic the program should look, how to parse the data and how to store it.

* **mqtt_name** is the metricname as defined via the regex. If the topic points to a JSON struct and not a single value, the names added via "dots" are the path inside the JSON struct to the value. So 'rpc.params.temperature:0.tC' means it's the topic which ends on 'rpc'. For more information about this, see the `find` examples of the [gojsonq](https://github.com/thedevsaddam/gojsonq) documentation.
* **name** is the keyword under which the data is stored in InfluxDB.
* **type** defines in which format the value stored, valid options are `float`, `int` and `string`. If the values are "on"/"off" or "true"/"false" or something similar, a mapping of the string to an integer (e.g. -1 for "N/A", 0 for "off" and 1 for "on") could be specified with **string_value_mapping**.
* **unit** will be stored as 'tag' in the database.

## Environment Variables

Having the login details in the config file runs the risk of publishing them to a version control system. To avoid this, you can supply these parameters via environment variables. mqtt-exporter will look for MQTT_USER and MQTT_PASSWORD in the local environment at startup.

### Example usage with container

```bash
  sudo podman run -e MQTT_USER="user" -e MQTT_PASSWORD="password" ...
```

### Example usage with kubernetes or podman kube play

```yaml
...
spec:
  containers:
  - name: mqtt-exporter
    image: <image>
    env:
    - name: MQTT_USER
      value: <username>
    - name: MQTT_PASSWORD
      value: <password>
...
```
## Liveness and readiness probes

This liveness and readiness health checks are needed if the service runs in Kubernetes. The livness probe tells kubernetes that the application is alive, if the service does not answer, the service will be restarted. The readiness probe tells kubernetes, when the container is ready to serve traffic.

The endpoints are:

* *IP:Port*/healthz for the liveness probe
* *IP:Port*/readyz for the readiness probe


The **IP:Port** will be defined with the `health_check` option in the configuration file. If this config variable is not set, the health check stay disabled.
