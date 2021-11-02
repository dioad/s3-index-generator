module github.com/dioad/s3-index-generator

go 1.16

replace github.com/fclairamb/afero-s3 v0.3.0 => github.com/patdowney/afero-s3 v0.3.1-0.20210403221449-e9d502439520

require (
	github.com/aws/aws-lambda-go v1.27.0
	github.com/aws/aws-sdk-go v1.41.16
	github.com/fclairamb/afero-s3 v0.3.0
	github.com/spf13/afero v1.6.0
)
