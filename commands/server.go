package main

import (
	"github.com/Lavos/mdhost"
	"github.com/Lavos/casket/contentstorers"
	"github.com/Lavos/casket/filers"
	"os"
	"io/ioutil"
)

func main () {
	r := contentstorers.NewRedis("localhost", "casket", 6379)
	f := filers.NewRedis("localhost", "casket", 6379)

	b, _ := ioutil.ReadAll(os.Stdin)
	s := mdhost.NewServer(r, f, ":8035", b)
	s.Run()
}
