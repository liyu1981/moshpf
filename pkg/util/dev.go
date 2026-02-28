package util

import "os"

func IsDev() bool {
	return os.Getenv("APP_ENV") == "dev"
}
