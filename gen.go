//go:generate mockgen -package mock -source internal/wrap/stream.go -destination internal/mock/stream.go
//go:generate mockgen -package mock -source internal/wrap/conn.go -destination internal/mock/conn.go
//go:generate mockgen -package mock -source internal/wrap/host.go -destination internal/mock/host.go
package pcp
