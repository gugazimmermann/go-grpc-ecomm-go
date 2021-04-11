#!/bin/bash

protoc ecommpb/ecomm.proto --go_out=plugins=grpc:.
