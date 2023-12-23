package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"gopkg.in/mcuadros/go-syslog.v2"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

const DEFAULT_CTX_TIMEOUT = 10 * time.Second
const DEFAULT_MAX_LINES = 5000
const DEFAULT_MAX_TIME = 300 * time.Second

func randSeq(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Sends syslog entries to a s3 bucket
// GZ Compressed files every 300 seconds or every 10K lines
type s3Buffer struct {
	buf        bytes.Buffer
	mutex      sync.Mutex
	Verbose    bool
	entries    int
	lastEntry  time.Time
	firstEntry time.Time
	awsService s3.S3
	awsSession session.Session
	awsContext context.Context
	bucket     string
	MaxTime    time.Duration
	MaxLines   int
}

func (this *s3Buffer) compressedWrite(text string) error {
	data := []byte(text)
	gw, err := gzip.NewWriterLevel(&this.buf, gzip.BestCompression)
	defer gw.Close()
	gw.Write(data)
	return err
}

// Create a new instance of the service's client with a Session.
func (buffer *s3Buffer) Init(endpoint, bucket, region, key, secret string) {
	buffer.Verbose = false
	buffer.bucket = bucket
	buffer.awsSession = *session.Must(session.NewSession())
	config := aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
		Endpoint:                       aws.String(endpoint),
		Region:                         aws.String(region),
		Credentials:                    credentials.NewStaticCredentials(key, secret, ""),
	}
	buffer.awsContext = context.Background()
	buffer.awsService = *s3.New(&buffer.awsSession, &config)
	buffer.MaxLines = DEFAULT_MAX_LINES
	buffer.MaxTime = DEFAULT_MAX_TIME
}

func (buffer *s3Buffer) Add(text string) error {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	// Initialize timers if needed
	if buffer.buf.Len() == 0 {
		buffer.firstEntry = time.Now()
	}
	// Write text to the buffer
	err := buffer.compressedWrite(text)
	if err != nil {
		return err
	} else {
		buffer.entries += 1
		buffer.lastEntry = time.Now()
	}
	// Check if buffer needs to be sent
	if buffer.entries >= buffer.MaxLines {
		if buffer.Verbose {
			log.Println("Maximum number of lines reached, sending logs")
		}
		buffer.Send()
	} else if time.Now().Sub(buffer.firstEntry).Seconds() > buffer.MaxTime.Seconds() && buffer.entries != 0 {
		if buffer.Verbose {
			log.Println("Max time reached, sending logs")
		}
		buffer.Send()
	}
	return nil
}

func (buffer *s3Buffer) Send() {
	filename := fmt.Sprintf("%s_%s_%s.log.gz",
		buffer.firstEntry.Format("20060102T150405Z"),
		buffer.lastEntry.Format("20060102T150405Z"),
		randSeq(8),
	)
	if buffer.Verbose {
		log.Printf("Sending gzip file: %s\n", filename)
	}

	var cancelFn func()
	buffer.awsContext, cancelFn = context.WithTimeout(buffer.awsContext, DEFAULT_CTX_TIMEOUT)
	// Ensure the context is canceled to prevent leaking.
	// See context package for more information, https://golang.org/pkg/context/
	if cancelFn != nil {
		defer cancelFn()
	}

	// Uploads the object to S3. The Context will interrupt the request if the timeout expires.
	_, err := buffer.awsService.PutObjectWithContext(buffer.awsContext, &s3.PutObjectInput{
		Bucket: aws.String(buffer.bucket),
		Key:    aws.String(filename),
		Body:   bytes.NewReader(buffer.buf.Bytes()),
	})
	buffer.buf.Reset()
	buffer.entries = 0
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			// If the SDK can determine the request or retry delay was canceled
			// by a context the CanceledErrorCode error code will be returned.
			log.Printf("Upload canceled due to timeout, %v\n", err)
		} else {
			log.Printf("Failed to upload object, %v\n", err)
		}
	} else {
		log.Printf("Successfully sent log to %s/%s\n", buffer.bucket, filename)
	}
}

func GetEnvStr(name, value string) string {
	if os.Getenv(name) != "" {
		return os.Getenv(name)
	}
	return value
}

func main() {
	rand.Seed(time.Now().UnixNano())

	bucket := flag.String("bucket", GetEnvStr("LOGFWRD_BUCKET", ""), "Name of the S3 bucket where syslog messages are stored")
	listenAddr := flag.String("listen", GetEnvStr("LOGFWRD_LISTEN", ":5014"), "Address for the syslog daemon to listen on")
	region := flag.String("region", GetEnvStr("LOGFWRD_REGION", "auto"), "Region where the S3 bucket is located")
	endpoint := flag.String("endpoint", GetEnvStr("LOGFWRD_ENDPOINT", ""), "URL of the S3 bucket endpoint")
	secret := flag.String("secret", GetEnvStr("LOGFWRD_SECRET", ""), "Secret key for accessing the S3 bucket")
	accessKey := flag.String("key", GetEnvStr("LOGFWRD_KEY", ""), "Access key for accessing the S3 bucket")
	maxLines := flag.String("max-records", GetEnvStr("LOGFWRD_MAX_RECORDS", "5000"), "Maximum number of log lines to deliver per batch")
	maxTime := flag.String("max-interval", GetEnvStr("LOGFWRD_MAX_INTERVAL", "5m"), "Maximum time interval between log deliveries")
	verbose := flag.Bool("verbose", false, "Specifies whether log messages should be shown or not")
	flag.Parse()

	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	buffer := s3Buffer{}
	buffer.Init(*endpoint, *bucket, *region, *accessKey, *secret)
	buffer.Verbose = *verbose

	var err error
	buffer.MaxTime, err = time.ParseDuration(*maxTime)
	if err != nil {
		log.Panicf("Error parsing <%s>: %v", *maxTime, err)
	}
	buffer.MaxLines, err = strconv.Atoi(*maxLines)
	if err != nil {
		log.Panicf("Error parsing <%s>: %v", *maxLines, err)
	}

	server := syslog.NewServer()
	server.SetFormat(syslog.Automatic)
	server.SetHandler(handler)
	server.ListenUDP(*listenAddr)
	server.Boot()
	if *verbose {
		log.Printf("Syslog server listening at %s\n", *listenAddr)
	}

	go func(channel syslog.LogPartsChannel) {
		for logParts := range channel {
			delete(logParts, "tls_peer")
			jsonString, err := json.Marshal(logParts)
			text := fmt.Sprintf("%s", jsonString)
			if err != nil {
				log.Printf("Failed to create json from syslog: %v\n", err)
				continue
			}
			err = buffer.Add(text + "\n")
			if err != nil {
				log.Printf("Failed to add syslog message to buffer: %v\n", err)
				continue
			}
		}
	}(channel)
	server.Wait()
}
