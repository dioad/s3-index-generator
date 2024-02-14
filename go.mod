module github.com/dioad/s3-index-generator

go 1.21

replace github.com/fclairamb/afero-s3 v0.3.0 => github.com/patdowney/afero-s3 v0.3.2

require (
	github.com/aws/aws-lambda-go v1.43.0
	github.com/aws/aws-sdk-go v1.50.18
	github.com/aws/aws-xray-sdk-go v1.8.3
	github.com/cenkalti/backoff/v3 v3.2.2
	github.com/coreos/go-semver v0.3.1
	github.com/fclairamb/afero-s3 v0.3.0
	github.com/spf13/afero v1.11.0
	github.com/stretchr/testify v1.8.0
	golang.org/x/sync v0.6.0
)

require (
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240102182953-50ed04b92917 // indirect
	google.golang.org/grpc v1.60.1 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
