package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	out := make([]rune, n)
	for i := range out {
		out[i] = letters[rand.Intn(len(letters))]
	}
	return string(out)
}

// Sends syslog entries to a s3 bucket
// GZ Compressed files every 300 seconds or every 10K lines
type s3Buffer struct {
	data       bytes.Buffer
	Verbose    bool
	entries    int
	lastEntry  time.Time
	firstEntry time.Time
	awsService s3.S3
	awsSession session.Session
	bucket     string
	MaxTime    time.Duration
	MaxLines   int
	Tag        string
}

func (buffer *s3Buffer) compressedWrite(text string) error {
	data := []byte(text)
	gw, err := gzip.NewWriterLevel(&buffer.data, gzip.BestCompression)
	if err != nil {
		return err
	}
	defer gw.Close()
	gw.Write(data)
	return nil
}

// Create a new instance of the service's client with a Session.
func (buffer *s3Buffer) Init(endpoint, bucket, region, key, secret string) {
	buffer.Verbose = false
	buffer.Tag = ""
	buffer.bucket = bucket
	buffer.awsSession = *session.Must(session.NewSession())
	config := aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
		Endpoint:                       aws.String(endpoint),
		Region:                         aws.String(region),
		Credentials:                    credentials.NewStaticCredentials(key, secret, ""),
	}
	buffer.awsService = *s3.New(&buffer.awsSession, &config)
	buffer.MaxLines = DEFAULT_MAX_LINES
	buffer.MaxTime = DEFAULT_MAX_TIME
}

func (buffer *s3Buffer) Add(text string) error {
	// Initialize timers if needed
	if buffer.data.Len() == 0 {
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
	} else if time.Since(buffer.firstEntry).Seconds() > buffer.MaxTime.Seconds() && buffer.entries != 0 {
		if buffer.Verbose {
			log.Printf("Max time reached, sending logs (%f > %f)\n", time.Since(buffer.firstEntry).Seconds(), buffer.MaxTime.Seconds())
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

	awsContext := context.Background()
	var cancelFn func()
	awsContext, cancelFn = context.WithTimeout(awsContext, DEFAULT_CTX_TIMEOUT)
	// Ensure the context is canceled to prevent leaking.
	// See context package for more information, https://golang.org/pkg/context/
	if cancelFn != nil {
		defer cancelFn()
	}

	headers := map[string]string{}
	if buffer.Tag != "" {
		headers = map[string]string{"x-amz-meta-tag": buffer.Tag}
	}
	// Uploads the object to S3. The Context will interrupt the request if the timeout expires.
	_, err := buffer.awsService.PutObjectWithContext(
		awsContext,
		&s3.PutObjectInput{
			Bucket: aws.String(buffer.bucket),
			Key:    aws.String(filename),
			Body:   bytes.NewReader(buffer.data.Bytes()),
		},
		request.WithSetRequestHeaders(headers),
	)
	buffer.data.Reset()
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
