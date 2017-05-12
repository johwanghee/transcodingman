FROM golang:1.8.1-alpine

RUN apk add --update ffmpeg

WORKDIR /go/src/transcodingman
COPY . .

RUN go install -v

CMD ["transcodingman"]
