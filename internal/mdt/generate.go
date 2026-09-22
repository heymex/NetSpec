package mdt

//go:generate protoc --proto_path=../../proto --go_out=../.. --go_opt=module=github.com/netspec/netspec --go-grpc_out=../.. --go-grpc_opt=module=github.com/netspec/netspec telemetry_bis.proto mdt_dialout.proto
