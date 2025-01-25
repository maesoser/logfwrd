package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"gopkg.in/mcuadros/go-syslog.v2"
)

const DEFAULT_CTX_TIMEOUT = 10 * time.Second
const DEFAULT_MAX_LINES = 5000
const DEFAULT_MAX_TIME = 30 * time.Second

func GetEnvStr(name, value string) string {
	if os.Getenv(name) != "" {
		return os.Getenv(name)
	}
	return value
}

func checkEmptyFlags(params map[string]*string) {
	emptyValues := false
	for name, value := range params {
		if *value == "" {
			log.Printf("Error: %s is empty", name)
			emptyValues = true
		}
	}
	if emptyValues {
		os.Exit(-1)
	}
}

func main() {

	// Common Options
	listenAddr := flag.String("listen", GetEnvStr("LOGFWRD_LISTEN", ":5014"), "Address for the syslog daemon to listen on")
	verbose := flag.Bool("verbose", false, "Specifies whether debug messages should be shown or not")
	endpoint := flag.String("endpoint", GetEnvStr("LOGFWRD_ENDPOINT", ""), "URL of the S3 bucket endpoint")
	mode := flag.String("mode", GetEnvStr("LOGFWRD_MODE", "s3"), "Mode of operation (s3 or http)")
	tag := flag.String("tag", GetEnvStr("LOGFWRD_TAG", ""), "Optional metadata string attached to the delivered files")

	// HTTP export options
	auth := flag.String("auth", GetEnvStr("LOGFWRD_AUTH_HEADER", ""), "Authorization header for accessing the HTTP endpoint")

	// S3 export options
	bucket := flag.String("bucket", GetEnvStr("LOGFWRD_BUCKET", ""), "Name of the S3 bucket where syslog messages are stored")
	secret := flag.String("secret", GetEnvStr("LOGFWRD_SECRET", ""), "Secret key for accessing the S3 bucket")
	accessKey := flag.String("key", GetEnvStr("LOGFWRD_KEY", ""), "Access key for accessing the S3 bucket")
	region := flag.String("region", GetEnvStr("LOGFWRD_REGION", "auto"), "Region where the S3 bucket is located")
	maxLines := flag.String("max-records", GetEnvStr("LOGFWRD_MAX_RECORDS", "5000"), "Maximum number of log lines to deliver per batch")
	maxTime := flag.String("max-interval", GetEnvStr("LOGFWRD_MAX_INTERVAL", "60s"), "Maximum time interval between log deliveries")

	flag.Parse()

	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)
	server := syslog.NewServer()
	server.SetFormat(syslog.Automatic)
	server.SetHandler(handler)
	server.ListenUDP(*listenAddr)
	server.Boot()
	if *verbose {
		log.Printf("Syslog server listening at %s\n", *listenAddr)
	}

	var s3Forwarder *s3Buffer
	var httpForwarder *httpBuffer

	if *mode == "http" {
		log.Println("Sending logs to HTTP endpoint")
		buffer := &httpBuffer{}
		buffer.Init(*endpoint, *auth)
		buffer.Verbose = *verbose
		buffer.Tag = *tag
		go func(channel syslog.LogPartsChannel) {
			for logParts := range channel {
				delete(logParts, "tls_peer")
				jsonString, err := json.Marshal(logParts)
				if err != nil {
					log.Printf("Failed to create json from syslog: %v\n", err)
					continue
				}
				err = buffer.Add(string(jsonString) + "\n")
				if err != nil {
					log.Printf("Failed to add syslog message to buffer: %v\n", err)
					continue
				}
			}
		}(channel)
	} else if *mode == "s3" {
		log.Println("Sending logs to S3 compatible endpoint")
		checkEmptyFlags(
			map[string]*string{
				"Bucket Name":     bucket,
				"S3 endpoint URL": endpoint,
				"S3 Secret key":   secret,
				"S3 Access Key":   accessKey,
			})

		buffer := &s3Buffer{}
		buffer.Init(*endpoint, *bucket, *region, *accessKey, *secret)
		buffer.Verbose = *verbose
		buffer.Tag = *tag

		var err error
		buffer.MaxTime, err = time.ParseDuration(*maxTime)
		if err != nil {
			log.Panicf("Error parsing <%s>: %v", *maxTime, err)
		}
		buffer.MaxLines, err = strconv.Atoi(*maxLines)
		if err != nil {
			log.Panicf("Error parsing <%s>: %v", *maxLines, err)
		}
	} else {
		log.Println("Error: mode is empty")
		os.Exit(-1)
	}

	go func(channel syslog.LogPartsChannel) {
		for logParts := range channel {
			delete(logParts, "tls_peer")
			jsonString, err := json.Marshal(logParts)
			if err != nil {
				log.Printf("Failed to create json from syslog: %v\n", err)
				continue
			}
			if *mode == "s3" {
				err = s3Forwarder.Add(string(jsonString) + "\n")
			} else {
				err = httpForwarder.Add(string(jsonString) + "\n")
			}
			if err != nil {
				log.Printf("Failed to add syslog message to buffer: %v\n", err)
				continue
			}
		}
	}(channel)
	server.Wait()
}
