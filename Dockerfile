FROM golang:1.10-alpine as builder

WORKDIR /go/src/github.com/rchampourlier/kaizenizer-source-jira
COPY . /go/src/github.com/rchampourlier/kaizenizer-source-jira

RUN apk --no-cache add git
RUN go get

RUN go build -o kaizenizer-source-jira

FROM alpine:3.7
RUN apk --no-cache add ca-certificates
COPY --from=builder /go/src/github.com/rchampourlier/kaizenizer-source-jira/kaizenizer-source-jira /kaizenizer-source-jira
