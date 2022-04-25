module github.com/dioad/s3-index-generator

go 1.18

replace github.com/fclairamb/afero-s3 v0.3.0 => github.com/patdowney/afero-s3 v0.3.1-0.20220101201844-c459c762e51b

require (
	github.com/aws/aws-lambda-go v1.31.1
	github.com/aws/aws-sdk-go v1.44.0
	github.com/fclairamb/afero-s3 v0.3.0
	github.com/spf13/afero v1.8.2
)

require (
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	golang.org/x/text v0.3.7 // indirect
)
