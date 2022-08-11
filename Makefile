proto:
	protoc -I=${PWD} \
		   --go_out=${PWD} \
		   --go-grpc_out=. --go-grpc_opt=paths=source_relative \
		   --go_opt=paths=source_relative \
		   ${PWD}/pkg/message/proto/message.proto

server:
	go build -o server cmd/server/*.go

client:
	go build -o client cmd/client-ui/*.go
