//go:generate mockgen -package mock -source internal/wrap/stream.go -destination internal/mock/stream.go
//go:generate mockgen -package mock -source internal/wrap/conn.go -destination internal/mock/conn.go
//go:generate mockgen -package mock -source internal/wrap/host.go -destination internal/mock/host.go
//go:generate mockgen -package mock -source internal/wrap/peerstore.go -destination internal/mock/peerstore.go
//go:generate mockgen -package mock -source internal/wrap/mdns.go -destination internal/mock/mdns.go
//go:generate mockgen -package mock -source internal/wrap/os.go -destination internal/mock/os.go
//go:generate mockgen -package mock -source internal/wrap/xdg.go -destination internal/mock/xdg.go
package pcp
