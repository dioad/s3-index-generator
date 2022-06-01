module github.com/dioad/s3-index-generator

go 1.18

replace github.com/fclairamb/afero-s3 v0.3.0 => github.com/patdowney/afero-s3 v0.3.1-0.20220101201844-c459c762e51b

require (
	github.com/aws/aws-lambda-go v1.32.0
	github.com/aws/aws-sdk-go v1.44.26
	github.com/aws/aws-xray-sdk-go v1.7.0
	github.com/fclairamb/afero-s3 v0.3.0
	github.com/spf13/afero v1.8.2
)

require (
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.15.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.36.0 // indirect
	golang.org/x/net v0.0.0-20220425223048-2871e0cb64e4 // indirect
	golang.org/x/sys v0.0.0-20220429233432-b5fbb4746d32 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20220429170224-98d788798c3e // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)
