proto:
	protoc -I=${PWD} --go_out=${PWD} ${PWD}/pkg/message/proto/message.proto

server:
	go build -o server cmd/server/*.go

client:
	go build -o client cmd/client-ui/*.go
