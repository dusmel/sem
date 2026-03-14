package app

import "sem/internal/config"

type App struct {
	Paths Paths
}

func New() (*App, error) {
	paths, err := Resolve()
	if err != nil {
		return nil, err
	}

	return &App{Paths: paths}, nil
}

func (a *App) LoadConfig() (config.Config, error) {
	return config.Load(a.Paths.ConfigPath)
}
