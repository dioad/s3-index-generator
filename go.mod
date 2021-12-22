module github.com/dioad/s3-index-generator

go 1.16

replace github.com/fclairamb/afero-s3 v0.3.0 => github.com/patdowney/afero-s3 v0.3.1-0.20211124214926-b093ec0afe92

require (
	github.com/aws/aws-lambda-go v1.27.0
	github.com/aws/aws-sdk-go v1.42.19
	github.com/fclairamb/afero-s3 v0.3.0
	github.com/spf13/afero v1.7.0
)
