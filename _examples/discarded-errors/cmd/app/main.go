// Command app is a tiny user store CLI used to exercise Sheriff's discarded
// error analysis and remediation.
package main

import (
	"fmt"

	"example/discarded-errors/internal/store"
)

func main() {
	u := store.User{Name: "arnold"}
	fmt.Println(string(store.Save(u)))

	if _, err := store.Load([]byte(`{"Name": "arnold"}`)); err != nil {
		fmt.Println("load:", err)
	}
}
