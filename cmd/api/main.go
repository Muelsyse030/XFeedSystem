package main

import (
	"XFeedSystem/configs"
	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/routers"
	"fmt"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	db := configs.InitDB(cfg.MySQL.DSN)
	r := routers.SetupRouter(db, *cfg)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if err := r.Run(addr); err != nil {
		panic(err)
	}
}
