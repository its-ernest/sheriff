// Command maybe-nil illustrates a potential nil dereference.
//
// lookup returns nil for a non-positive id. It is called with an id that
// Sheriff cannot establish (it comes from the environment), so the nil path is
// possible. Sheriff reports a potential finding: the dereference may be nil.
//
// Run with no ID (or a non-positive one) to see the panic:
//
//	go run ./_examples/nilptr-cases/maybe-nil
//
// Run with a positive ID to see it succeed:
//
//	ID=10 go run ./_examples/nilptr-cases/maybe-nil
package main

import (
	"fmt"
	"os"
	"strconv"
)

type User struct{ Name string }

func lookup(id int) *User {
	if id > 0 {
		return &User{Name: "arnold"}
	}
	return nil
}

func main() {
	id, _ := strconv.Atoi(os.Getenv("ID"))
	user := lookup(id)
	fmt.Println(user.Name)
}
