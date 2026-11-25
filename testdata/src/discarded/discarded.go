package discarded

import (
	"encoding/json"
	"fmt"
)

type User struct{ Name string }

func marshalDiscarded(user User) {
	data, _ := json.Marshal(user) // want `discarded error: json\.Marshal\(user\)`
	fmt.Println(string(data))
}

func assignDiscarded(user User) {
	var data []byte
	data, _ = json.Marshal(user) // want `discarded error: json\.Marshal\(user\)`
	fmt.Println(string(data))
}

func marshalHandled(user User) {
	data, err := json.Marshal(user)
	if err != nil {
		return
	}
	fmt.Println(string(data))
}

func mapLookup(m map[string]int) {
	v, _ := m["k"]
	fmt.Println(v)
}

func twoValues() {
	x, _ := 1, 2
	fmt.Println(x)
}
