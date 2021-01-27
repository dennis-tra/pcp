//go:generate mockgen -package mock -source internal/app/ioutil.go -destination internal/mock/ioutil.go
//go:generate mockgen -package mock -source internal/app/xdg.go -destination internal/mock/xdg.go
//go:generate mockgen -package mock -source internal/app/p2p_discovery.go -destination internal/mock/p2p_discovery.go
//go:generate mockgen -package mock -source internal/app/time.go -destination internal/mock/time.go
package pcp
