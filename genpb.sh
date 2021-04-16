#!/bin/bash

protoc ecommpb/ecomm.proto --go_out=plugins=grpc:. --js_out=import_style=commonjs,binary:./frontend/src/ --grpc-web_out=import_style=typescript,mode=grpcwebtext:./frontend/src/