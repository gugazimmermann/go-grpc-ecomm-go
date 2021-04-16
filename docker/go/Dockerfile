FROM golang
WORKDIR /server
COPY ./main.go .
COPY ./.env .
COPY ./go.mod .
COPY ./go.sum .
COPY ./certs ./certs
COPY ./ecommpb ./ecommpb
RUN go build main.go
EXPOSE 50051
ENTRYPOINT ["./main"]
# ENTRYPOINT /server/main