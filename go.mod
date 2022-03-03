module github.com/dioad/s3-index-generator

go 1.16

replace github.com/fclairamb/afero-s3 v0.3.0 => github.com/patdowney/afero-s3 v0.3.1-0.20220101201844-c459c762e51b

require (
	github.com/aws/aws-lambda-go v1.28.0
	github.com/aws/aws-sdk-go v1.43.11
	github.com/fclairamb/afero-s3 v0.3.0
	github.com/spf13/afero v1.8.1
)
