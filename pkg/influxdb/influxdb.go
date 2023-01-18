package influxdb

import (
	"context"
	"fmt"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/influxdata/influxdb-client-go/v2"
)

const (
	defInfluxDBPort = "8086"
)

type InfluxDBConfig struct {
	Server       string `yaml:"server"`
	Port         string `yaml:"port"`
	Database     string `yaml:"database"`
	Organization string `yaml:"organizatin"`
}

func WriteEntry(client influxdb2.Client, config InfluxDBConfig, measurement string, tag map[string]string, field map[string]interface{}) error {
	writeAPI := client.WriteAPI(config.Organization, config.Database)
	// Get errors channel
	errorsCh := writeAPI.Errors()
	// Create go proc for reading and logging errors
	go func() {
		for err := range errorsCh {
			// XXX better way of error reporting
			fmt.Printf("Write error: %s\n", err.Error())
		}
	}()

	p := influxdb2.NewPoint(measurement, tag, field, time.Now())
	// write asynchronously
	writeAPI.WritePoint(p)

	return nil
}

func createDatabase(client influxdb2.Client, config *InfluxDBConfig) error {

	ctx := context.Background()
	// Get Buckets API client
	bucketsAPI := client.BucketsAPI()
	// Get organization that will own new bucket
	org, err := client.OrganizationsAPI().FindOrganizationByName(ctx, config.Organization)
	if err != nil {
		return err
	}
	// Create  a bucket with 1 day retention policy
	_, err = bucketsAPI.CreateBucketWithName(ctx, org, config.Database, domain.RetentionRule{EverySeconds: 0})
	if err != nil {
		return err
	}
	return nil
}

func ConnectInfluxDB(config *InfluxDBConfig) influxdb2.Client {
	// Create a new client using an InfluxDB server base URL and an
	// authentication token
	if len(config.Port) == 0 {
		config.Port = defInfluxDBPort
	}
	serverUrl := fmt.Sprintf("http://%s:%s",
		config.Server, config.Port)
	client := influxdb2.NewClient(serverUrl, "") // XXX token
	defer client.Close()

	// XXX Verify database exists, if not create it

	return client
}
