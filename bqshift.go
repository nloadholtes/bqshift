package main

import (
	"fmt"
	"github.com/uswitch/bqshift/bigquery"
	"github.com/uswitch/bqshift/redshift"
	"github.com/uswitch/bqshift/storage"
	"log"
)

type shifter struct {
	redshift *redshift.Client
	config   *Configuration
}

func (s *shifter) Run(table string, config *bigquery.Configuration) error {
	storageClient, err := storage.NewClient(config, s.config.CredentialsConfiguration.S3)
	if err != nil {
		return err
	}

	bq, err := bigquery.NewClient()
	exists, err := bq.DatasetExists(config.ProjectID, config.DatasetName)
	if err != nil {
		return fmt.Errorf("error checking dataset: %s", err.Error())
	}

	if !exists {
		return fmt.Errorf("dataset doesn't exist: %s", config.DatasetName)
	}

	if err != nil {
		return fmt.Errorf("error creating bigquery client: %s", err.Error())
	}

	log.Println("unloading to s3")
	result, err := s.redshift.Unload(table)
	if err != nil {
		return fmt.Errorf("error unloading: %s", err.Error())
	}
	log.Println("transferring to cloud storage")
	stored, err := storageClient.TransferToCloudStorage(result)
	if err != nil {
		return fmt.Errorf("error transferring to cloud storage: %s", err.Error())
	}

	sourceSchema, err := s.redshift.ExtractSchema(table)
	if err != nil {
		return fmt.Errorf("error extracting source schema: %s", err.Error())
	}
	destSchema, err := sourceSchema.ToBigQuerySchema()
	if err != nil {
		return fmt.Errorf("error translating redshift schema to bigquery: %s", err.Error())
	}

	log.Println("loading into bigquery")
	ref := bigquery.TableReference(config.ProjectID, config.DatasetName, table)
	spec := &bigquery.LoadSpec{
		TableReference: ref,
		BucketName:     stored.BucketName,
		ObjectPrefix:   stored.Prefix,
		Overwrite:      s.config.OverwriteBigQuery,
		Schema:         destSchema,
	}
	done, err := bq.LoadTable(spec)
	if err != nil {
		return fmt.Errorf("error loading data into table: %s", err.Error())
	}

	if s.config.WaitForLoad {
		result := <-done
		if result.Error != nil {
			return fmt.Errorf("error loading data into table: %s", result.Error.Error())
		}
	}

	return nil
}

func NewShifter(config *Configuration) (*shifter, error) {
	client, err := redshift.NewClient(config.CredentialsConfiguration.Redshift, config.CredentialsConfiguration.S3)
	if err != nil {
		return nil, err
	}

	return &shifter{client, config}, nil
}
