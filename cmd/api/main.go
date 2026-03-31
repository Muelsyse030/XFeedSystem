package main

import (
	"XFeedSystem/configs"
	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/routers"
	"fmt"
)

func main() {
	cfg, err := config.LoadConfig()
	db := configs.InitDB()
	if err != nil {
		panic(err)
	}
	r := routers.SetupRouter(db)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if err := r.Run(addr); err != nil {
		panic(err)
	}
}
