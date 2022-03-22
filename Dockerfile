# FROM golang:1.17.5-alpine AS builder
# LABEL stage=builder

# RUN apk add --no-cache git upx

# ENV GOPATH /go

# COPY ./ /go/src/
# WORKDIR /go/src/

# RUN echo $GOPATH
# RUN go get 
# RUN CGO_ENABLED=0 GOOS=linux go build . 
# RUN upx k8see-importer

# FROM busybox:1.35.0 AS builder
# ARG GOOS
# ARG GOARCH
# # RUN apk update && apk add --no-cache curl bash
# RUN wget  https://github.com/amacneil/dbmate/releases/download/v1.12.1/dbmate-${GOOS}-${GOARCH} && chmod +x dbmate-${GOOS}-${GOARCH} && mv dbmate-${GOOS}-${GOARCH} /usr/local/bin/dbmate



FROM scratch AS final
ARG GOOS
ARG GOARCH

COPY resources /
COPY k8see-importer /opt/k8see-importer/k8see-importer
COPY cots/dbmate-${GOOS}-${GOARCH} /usr/local/bin/dbmate
# COPY --from=builder /usr/local/bin/dbmate /usr/local/bin/dbmate
WORKDIR /opt/k8see-importer
USER k8see
CMD [ "/opt/k8see-importer/k8see-importer" ]
