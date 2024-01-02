package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-xray-sdk-go/xray"
)

func s3Session() *session.Session {
	sess := session.Must(
		session.NewSessionWithOptions(
			session.Options{
				SharedConfigState: session.SharedConfigEnable,
			},
		),
	)
	return sess
}

func s3Client(sess *session.Session) *s3.S3 {
	client := s3.New(sess, &aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
	})
	xray.AWS(client.Client)

	return client
}
