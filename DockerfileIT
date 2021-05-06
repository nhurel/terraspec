FROM golang:1.14.4

WORKDIR /terraspec
COPY . .

RUN go test ./integration_tests -tags=integrationtests