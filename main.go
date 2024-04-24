package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

func parseCurlCommand(curlCommand string) map[string]interface{} {
	// Simplify newlines and spaces for easier regex processing
	curlCommand = strings.Replace(curlCommand, "\\\n", " ", -1)
	curlCommand = strings.Replace(curlCommand, "\n", " ", -1)

	reMethodAndUrl := regexp.MustCompile(`--request (\w+) '([^']*)'`)
	reHeader := regexp.MustCompile(`--header '([^:]+): ([^']*)'`)
	reDataRaw := regexp.MustCompile(`--data-raw '(\{.*?\})'`)

	matches := reMethodAndUrl.FindStringSubmatch(curlCommand)
	method, extractedUrl := "GET", ""
	if len(matches) > 2 {
		method = matches[1]
		extractedUrl = matches[2]
	}

	if !strings.Contains(extractedUrl, "://") {
		extractedUrl = "http://" + extractedUrl // Assume HTTP if no scheme is given
	}

	parsedUrl, _ := url.Parse(extractedUrl)
	query := []map[string]interface{}{}
	for key, values := range parsedUrl.Query() {
		for _, value := range values {
			query = append(query, map[string]interface{}{
				"key":      key,
				"value":    value,
				"disabled": false,
			})
		}
	}

	headers := []map[string]string{}
	for _, match := range reHeader.FindAllStringSubmatch(curlCommand, -1) {
		headers = append(headers, map[string]string{
			"key":   match[1],
			"value": match[2],
		})
	}

	dataRawMatch := reDataRaw.FindStringSubmatch(curlCommand)
	rawData := ""
	mode := "none"
	if len(dataRawMatch) > 1 {
		rawData = dataRawMatch[1]
		mode = "raw"
	}
	fmt.Println(parsedUrl.RequestURI())
	return map[string]interface{}{
		"name": "Generated from Curl",
		"protocolProfileBehavior": map[string]interface{}{
			"disableBodyPruning": true,
		},
		"request": map[string]interface{}{
			"method": method,
			"header": headers,
			"body": map[string]interface{}{
				"mode": mode,
				"raw":  rawData,
			},
			"url": map[string]interface{}{
				"raw":      parsedUrl.String(),
				"protocol": parsedUrl.Scheme,
				"host":     []string{parsedUrl.Hostname()},
				"port":     parsedUrl.Port(),
				"path":     []string{strings.TrimLeft(parsedUrl.Path, "/")},
				"query":    query,
			},
		},
		"response": []interface{}{},
	}
}

type PostmanCollection struct {
	Info struct {
		PostmanID  string `json:"_postman_id"`
		Name       string `json:"name"`
		Schema     string `json:"schema"`
		ExporterID string `json:"_exporter_id"`
	} `json:"info"`
	Items []interface{} `json:"item"`
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	// Correctly format the directory path to include "keploy"
	keployDir := filepath.Join(cwd, "keploy")

	// Check if the directory exists
	if _, err := os.Stat(keployDir); os.IsNotExist(err) {
		fmt.Println("Keploy directory does not exist in the current working directory.")
		return
	}
	dir, err := ReadDir(keployDir, fs.FileMode(os.O_RDONLY))
	if err != nil {
		fmt.Println("creating a folder for the keploy generated testcases", zap.Error(err))
		return
	}

	files, err := dir.ReadDir(0)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	collection := PostmanCollection{
		Info: struct {
			PostmanID  string `json:"_postman_id"`
			Name       string `json:"name"`
			Schema     string `json:"schema"`
			ExporterID string `json:"_exporter_id"`
		}{
			PostmanID:  "b8623e1b69-224e-4ff3-801c-a95d480859bd",
			Name:       "Atlantis",
			Schema:     "https://schema.getpostman.com/json/collection/v2.0.0/collection.json",
			ExporterID: "132182772",
		},
	}
	for _, v := range files {
		if strings.Contains(v.Name(), "test-set") {
			testsDir := filepath.Join(keployDir, v.Name(), "tests")
			if _, err := os.Stat(testsDir); os.IsNotExist(err) {
				fmt.Println("No 'tests' subfolder in:", v.Name())
				continue
			}
			// Read the "tests" subfolder
			testFiles, err := ioutil.ReadDir(testsDir)
			if err != nil {
				fmt.Println("Error reading 'tests' directory:", err)
				continue
			}
			for _, testFile := range testFiles {
				if filepath.Ext(testFile.Name()) == ".yaml" {
					filePath := filepath.Join(testsDir, testFile.Name())

					// Read the YAML file
					data, err := os.ReadFile(filePath)
					if err != nil {
						fmt.Println("Error reading file:", err)
						continue
					}

					// Parse the YAML file (assuming it's a map for simplicity)
					var yamlData map[string]interface{}
					err = yaml.Unmarshal(data, &yamlData)
					if err != nil {
						fmt.Println("Error parsing YAML:", err)
						continue
					}
					if curl, ok := yamlData["curl"].(string); ok {
						requestJSON := parseCurlCommand(curl)
						collection.Items = append(collection.Items, requestJSON)
					}
				}
			}

		}
	}
	outputData, err := json.MarshalIndent(collection, "", "    ")
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}

	if err := os.WriteFile("output.json", outputData, 0644); err != nil {
		fmt.Println("Error writing JSON to file:", err)
		return
	}

	fmt.Println("Data written to output.json")

}

func ReadDir(path string, fileMode fs.FileMode) (*os.File, error) {
	dir, err := os.OpenFile(path, os.O_RDONLY, fileMode)
	if err != nil {
		return nil, err
	}
	return dir, nil
}
