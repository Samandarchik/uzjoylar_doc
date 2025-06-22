# Eski versiya (muammo tug‘diradi)
# FROM golang:1.21

# Yangi, mos versiya:
FROM golang:1.23

WORKDIR /app

COPY . .

RUN go build -o main .

CMD ["./main"]
