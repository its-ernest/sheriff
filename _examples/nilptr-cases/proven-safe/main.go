// Command proven-safe illustrates a call Sheriff can prove is safe.
//
// lookup returns nil only for a non-positive id. It is called with the constant
// 10, so the nil branch is unreachable and the result is non-nil. Sheriff stays
// silent, and running the program prints normally.
package main

import "fmt"

type User struct{ Name string }

func lookup(id int) *User {
	if id > 0 {
		return &User{Name: "arnold"}
	}
	return nil
}

func main() {
	user := lookup(10)
	fmt.Println(user.Name)
}
