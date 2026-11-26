# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /drover-api .

# No shell or package manager: the binary is its own healthcheck (-healthcheck).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /drover-api /drover-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/drover-api"]
