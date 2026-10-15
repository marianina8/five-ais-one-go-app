// This file is not part of the build, just for cleanup
package main

import (
	"os"
)

func main() {
	files := []string{
		"test_store.json",
		"test_store_visits.json",
		"test_store_all.json",
		"test_store_delete.json",
		"test_store_persist.json",
		"test_handler.json",
		"test_redirect.json",
		"test_list.json",
		"test_delete.json",
		"test_auth.json",
		"test_methods.json",
	}

	for _, file := range files {
		os.Remove(file)
	}
}