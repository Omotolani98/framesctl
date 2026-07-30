# syntax=docker/dockerfile:1

# ---------- Build stage ----------
FROM golang:1.26.2-alpine AS build

WORKDIR /src

# Cache dependencies separately from source so code edits don't re-download modules.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary: modernc.org/sqlite is pure Go, so CGO can stay off.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/framesrvr \
    ./cmd/framesrvr

# ---------- Runtime stage ----------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/framesrvr /framesrvr

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/framesrvr"]
