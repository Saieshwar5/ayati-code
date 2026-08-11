package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/config"
	"github.com/Saieshwar5/ayati-code/internal/ui"
)

func ensureConfiguration(
	ctx context.Context,
	path string,
	force bool,
	console *ui.Console,
) (config.Values, error) {
	values, err := config.Load(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return config.Values{}, fmt.Errorf("load configuration: %w", err)
	}
	if exists && !force {
		return values, nil
	}
	console.Setup(path, exists)
	keyLabel := "Fireworks API key: "
	modelLabel := "Fireworks model: "
	if exists {
		keyLabel = "Fireworks API key (leave blank to keep saved key): "
		modelLabel = fmt.Sprintf("Fireworks model [%s]: ", values.Model)
	}
	key, err := console.Secret(ctx, keyLabel)
	if err != nil {
		return config.Values{}, fmt.Errorf("read Fireworks API key: %w", err)
	}
	model, err := console.Value(ctx, modelLabel)
	if err != nil {
		return config.Values{}, fmt.Errorf("read Fireworks model: %w", err)
	}
	if strings.TrimSpace(key) != "" {
		values.FireworksAPIKey = key
	}
	if strings.TrimSpace(model) != "" {
		values.Model = model
	}
	if err := config.Save(path, values); err != nil {
		return config.Values{}, err
	}
	console.ConfigurationSaved(path)
	return config.Load(path)
}
