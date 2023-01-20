# mqtt-exporter
**MQTT Exporter - listens to MQTT topics and forwards them to InfluxDB**


This exporter listens to MQTT topics and stores them in an InfluxDB database. It takes the JSON message from the topics and translates this between the MQTT represenation and the format needed for database storage. How this translation is done is specified in the configuration file for mqtt-exporter, as you normally cannot adjust the messages IoT devices send. The IoT devices will push tehri metrics via MQTT to an MQTT Broker and this exporter subscribes to the broker and processes the rrecived messages.

```plaintext
 IoT Sensors -> publish -> MQTT Broker <- subcribed <- MQTT-Exporter -> stores -> InfluxDB
 ```

I wrote this exporter initial to export the power metering values of my Shelly Plug S devices and make them visible in Grafana.

## Assumptions about Messages and Topics

Currently the exporter only supports devices, which publish every metric in an own topic. For Shelly Plug S devices, the MQTT messages would look like:

```plaintext
shellies/shelly-plug-s1/relay/0/power 20.58
shellies/shelly-plug-s1/relay/0/energy 94736
shellies/shelly-plug-s1/relay/0 on
shellies/shelly-plug-s1/temperature 22.28
shellies/shelly-plug-s1/temperature_f 72.10
shellies/shelly-plug-s1/overtemperature 0
```

The device ID, which is used for the `measurement` field when writing the data into the database, is gotten by a regular expression (see `device_id_regex` in the configuration file) from the MQTT topic. This allows an arbitrary place of the device ID in the mqtt topic. For example the tasmota firmware pushes the telemetry data to the topic `tele/<deviceid>/SENSOR`, the Shelly Plug S uses `shellies/<deviceid>/SENSOR` while the Shelly Plus H&T uses `<deviceid>/events/rpc`.

The name for the metric (`metricname`) is extracted from the topic by the regular expression `metric_per_topic_regex`.

With my config file for Shelly Plug devices, the above MQTT messages would be converted to the following database points:

```
measurement: shelly-plug-s1, tags: {"unit": "Watt"}, field: {"power": 20.58}
measurement: shelly-plug-s1, tags: ["unit": "Watt/Minute"}, field: {"energy": 94736}
measurement: shelly-plug-s1, tags: {"unit": "C"}, field: {"temperature": 22.28}
measurement: shelly-plug-s1, tags: {}, field: {"switch": 2}
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
mqtt:
  # Required: The MQTT broker to connect to
  broker: 172.17.0.1
  # Optinal: Port of the MQTT broker
  # port: 1883
  # Optional: Username and Password for authenticating with the MQTT Server
  user: <username>
  password: <password>
  # Optional: Used to specify ClientID. The default is <hostname>-<pid>
  # client_id: somedevice
  # The Topic path to subscribe to. Be aware that you have to specify the
  # wildcard.
  topic_path: shellies/#
  # Optional: Regular expression to extract the device ID from the topic
  # path. The default regular expression, assumes that the last "element"
  # of the topic_path is the device id. The regular expression must contain
  # a named capture group with the name deviceid. For example the
  # expression for tasamota based sensors is "tele/(?P<deviceid>.*)/.*".
  # The default is:
  # device_id_regex: "(.*/)?(?P<deviceid>.*)"
  device_id_regex: "shellies/(?P<deviceid>.*?)/.*"
  # Optional: Expect a single metric to be published as the value on an
  # mqtt topic. This regex is used to extract the metric name from the
  # topic. Must contain a named group for `metricname`.
  metric_per_topic_regex: "shellies/.*/(?P<metricname>.*)"
  # The MQTT QoS level
  qos: 0
influxdb:
  server: defiant.thkukuk.de
  database: shellies
db_mapping:
  - mqtt_name: temperature
    name: temperature
    unit: C
    type: float
    # Optional: A map of strings for constant tags. This tags will always
    # be attached
    const_tags:
      reliable: false
      sensor: shelly-plug-s
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
```

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
