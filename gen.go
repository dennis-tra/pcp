//go:generate mockgen -package mock -source internal/wrap/ioutil.go -destination internal/mock/ioutil.go
//go:generate mockgen -package mock -source internal/wrap/xdg.go -destination internal/mock/xdg.go
//go:generate mockgen -package mock -source internal/wrap/time.go -destination internal/mock/time.go
//go:generate mockgen -package mock -source internal/wrap/dht.go -destination internal/mock/dht.go
//go:generate mockgen -package mock -source internal/wrap/manet.go -destination internal/mock/manet.go
//go:generate mockgen -package mock -source internal/wrap/discovery.go -destination internal/mock/discovery.go
package pcp
