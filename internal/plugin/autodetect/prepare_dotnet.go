package autodetect

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Configuration struct {
	XMLName xml.Name `xml:"configuration"`
	Config  *Config  `xml:"config"`
}

type Config struct {
	Add []Add `xml:"add"`
}

type Add struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

type dotnetPreparer struct{}

func newDotnetPreparer() *dotnetPreparer {
	return &dotnetPreparer{}
}

func (*dotnetPreparer) PrepareRepo(dir string) (string, error) {
	pathToCache := filepath.Join(dir, ".nuget", "packages")
	configPath := filepath.Join(dir, "nuget.config")
	updateConfig := false
	foundGlobalPackagesFolder := false

	//file not exists
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(configPath)
		if err != nil {
			return "", err
		}
		defer f.Close()

		configuration := Configuration{}
		marshalledConfig, err := xml.MarshalIndent(configuration, "", "  ")
		if err != nil {
			return "", err
		}

		err = os.WriteFile(configPath, marshalledConfig, 0644)
		if err != nil {
			return "", fmt.Errorf("failed to write nuget.config: %w", err)
		}
	}

	//file exists
	file, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read the file contents
	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the XML
	var configuration Configuration
	err = xml.Unmarshal(data, &configuration)
	if err != nil {
		return "", fmt.Errorf("failed to parse XML: %w", err)
	}

	if configuration.Config == nil {
		configuration.Config = &Config{}
		updateConfig = true
	}

	for _, add := range configuration.Config.Add {
		if add.Key == "globalPackagesFolder" {
			if add.Value != pathToCache && add.Value != filepath.Join(".nuget", "packages") {
				return "", fmt.Errorf("unsupported custom globalPackagesFolder value")
			}
			foundGlobalPackagesFolder = true
		}
	}

	if !foundGlobalPackagesFolder {
		// Add the globalPackagesFolder value
		configuration.Config.Add = append(configuration.Config.Add, Add{
			Key:   "globalPackagesFolder",
			Value: pathToCache,
		})
		updateConfig = true
	}

	if updateConfig {
		writeConfigurationToFile(configPath, configuration)
	}

	return pathToCache, nil
}

func writeConfigurationToFile(filePath string, config Configuration) error {
	updatedData, err := xml.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated nuget.config: %w", err)
	}
	err = os.WriteFile(filePath, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated nuget.config: %w", err)
	}
	return nil
}
