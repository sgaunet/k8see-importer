FROM golang:1.17.5-alpine AS builder
LABEL stage=builder

RUN apk add --no-cache git upx

ENV GOPATH /go

COPY ./ /go/src/
WORKDIR /go/src/

RUN echo $GOPATH
RUN go get 
RUN CGO_ENABLED=0 GOOS=linux go build . 
RUN upx k8see-importer



FROM alpine:3.15.0 AS final
LABEL maintainer="Sylvain Gaunet"
LABEL description=""

RUN apk update && apk add --no-cache curl bash
RUN curl -LO  https://github.com/amacneil/dbmate/releases/download/v1.12.1/dbmate-linux-amd64 && chmod +x dbmate-linux-amd64 && mv dbmate-linux-amd64 /usr/local/bin/dbmate

RUN addgroup -S k8see_group -g 1000 && adduser -S k8see -G k8see_group --uid 1000

WORKDIR /opt/k8see-importer
COPY --from=builder /go/src/k8see-importer .
COPY entrypoint.sh /opt/k8see-importer/entrypoint.sh 
COPY db /opt/k8see-importer/db
RUN chmod +x /opt/k8see-importer/entrypoint.sh && touch /opt/k8see-importer/conf.yaml && chmod 777 /opt/k8see-importer/conf.yaml

USER k8see

ENTRYPOINT ["/opt/k8see-importer/entrypoint.sh"]
CMD [ "/opt/k8see-importer/k8see-importer","-f","/opt/k8see-importer/conf.yaml" ]
