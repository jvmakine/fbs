package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the merged configuration from all fbs.conf.json files
type Config struct {
	Discoverers map[string]json.RawMessage `json:"discoverers"`
}

// DiscovererConfig represents configuration for a specific discoverer
type DiscovererConfig interface {
	// GetDiscovererID returns the unique ID for this discoverer
	GetDiscovererID() string
}

// ArtifactDownloadConfig represents configuration for artifact downloads
type ArtifactDownloadConfig struct {
	Repositories []string `json:"repositories"`
}

// GetDiscovererID returns the discoverer ID for artifact downloads
func (c *ArtifactDownloadConfig) GetDiscovererID() string {
	return "artifact-download"
}

// LoadConfiguration loads and merges all fbs.conf.json files from the directory hierarchy
func LoadConfiguration(startDir string) (*Config, error) {
	config := &Config{
		Discoverers: make(map[string]json.RawMessage),
	}
	
	// Walk up the directory hierarchy looking for fbs.conf.json files
	currentDir := startDir
	var configFiles []string
	
	for {
		configPath := filepath.Join(currentDir, "fbs.conf.json")
		if _, err := os.Stat(configPath); err == nil {
			configFiles = append(configFiles, configPath)
		}
		
		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			break
		}
		currentDir = parentDir
	}
	
	// Process config files from root to leaf (so leaf configs override parent configs)
	for i := len(configFiles) - 1; i >= 0; i-- {
		err := config.mergeConfigFile(configFiles[i])
		if err != nil {
			return nil, fmt.Errorf("failed to merge config file %s: %w", configFiles[i], err)
		}
	}
	
	return config, nil
}

// mergeConfigFile merges a single config file into the current configuration
func (c *Config) mergeConfigFile(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	
	var fileConfig Config
	err = json.Unmarshal(data, &fileConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	
	// Merge discoverer configurations
	for discovererID, discovererConfig := range fileConfig.Discoverers {
		c.Discoverers[discovererID] = discovererConfig
	}
	
	return nil
}

// GetDiscovererConfig retrieves configuration for a specific discoverer
func (c *Config) GetDiscovererConfig(discovererID string, result interface{}) error {
	rawConfig, exists := c.Discoverers[discovererID]
	if !exists {
		return fmt.Errorf("no configuration found for discoverer %s", discovererID)
	}
	
	err := json.Unmarshal(rawConfig, result)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config for discoverer %s: %w", discovererID, err)
	}
	
	return nil
}

// HasDiscovererConfig checks if configuration exists for a specific discoverer
func (c *Config) HasDiscovererConfig(discovererID string) bool {
	_, exists := c.Discoverers[discovererID]
	return exists
}