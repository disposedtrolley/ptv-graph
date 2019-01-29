package main

import (
	"encoding/csv"
	"fmt"
	"github.com/mholt/archiver"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var looseInputFiles = "./gtfs_in"
var consolidatedOutputFiles = "./gtfs_out"
var innerZipFileName = "google_transit.zip"
var validGTFSFileNames = []string{"agency", "calendar_dates", "calendar", "routes", "stop_times", "stops", "trips", "shapes"}

// GTFSRecord represents a GTFS record which has been read by walking the extracted
// input zip. The Type property denotes the kind of GTFS file residing at this path,
// valid values are those in the validGTFSFileNames array.
type GTFSRecord struct {
	Path     string
	Type     string
	Contents []string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Input .zip not provided. Usage: ./prepare-ptv-data <input.zip>")
		os.Exit(1)
	}

	inputPath := os.Args[1]

	err := extractPTVData(inputPath)
	if err != nil {
		log.Fatal(err)
	}

	var outputData = map[string][][]string{
		"agency":         [][]string{{"agency_id", "agency_name", "agency_url", "agency_timezone", "agency_lang"}},
		"calendar_dates": [][]string{{"service_id", "date", "exception_type"}},
		"calendar":       [][]string{{"service_id", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "start_date", "end_date"}},
		"routes":         [][]string{{"route_id", "agency_id", "route_short_name", "route_long_name", "route_type", "route_color", "route_text_color"}},
		"stop_times":     [][]string{{"trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence", "stop_headsign", "pickup_type", "drop_off_type", "shape_dist_traveled"}},
		"stops":          [][]string{{"stop_id", "stop_name", "stop_lat", "stop_lon"}},
		"trips":          [][]string{{"route_id", "service_id", "trip_id", "shape_id", "trip_headsign", "direction_id"}},
		"shapes":         [][]string{{"shape_id", "shape_pt_lat", "shape_pt_lon", "shape_pt_sequence", "shape_dist_traveled"}},
	}

	for record := range walkPTVData(looseInputFiles) {
		if !isGTFSRecordExisting(record, outputData[record.Type]) {
			outputData[record.Type] = append(outputData[record.Type], record.Contents)
		}
	}

	writeOutput(outputData, consolidatedOutputFiles, "txt")

	cleanup()
}

// Removes the temporary directories (gtfs_in and gtfs_out) created when
// the original files were extracted and the consolidated output was produced.
func cleanup() {
	err := os.RemoveAll(looseInputFiles)
	if err != nil {
		log.Printf("Error when deleting extracted input files: %s\n", err.Error())
	}

	err = os.RemoveAll(consolidatedOutputFiles)
	if err != nil {
		log.Printf("Error when deleting consolidated output files: %s\n", err.Error())
	}
}

// Writes each 2D string slice in the supplied map to its own CSV file, where
// the name of the file is the key of the map.
func writeOutput(data map[string][][]string, path string, ext string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.MkdirAll(path, os.ModePerm)
	}

	for k, v := range data {
		writeCSV(v, fmt.Sprintf("%s/%s.%s", path, k, ext))
	}

	archiver.Archive([]string{path}, fmt.Sprintf("%s.zip", path))
}

// Writes a 2D slice of strings to a CSV file.
func writeCSV(data [][]string, path string) {
	file, err := os.Create(path)

	if err != nil {
		log.Fatalf("Unable to create output file %s: %s\n", path, err.Error())
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, value := range data {
		err := writer.Write(value)
		if err != nil {
			log.Fatalf("Unable to write row to file: %s\n", err.Error())
		}
	}
}

// Returns whether a supplied GTFSRecord exists in a target array.
func isGTFSRecordExisting(rec GTFSRecord, targetArrays [][]string) bool {
	for _, arr := range targetArrays {
		if rec.Contents[0] == arr[0] {
			return true
		}
	}

	return false
}

// Returns whether a given filename is likely a GTFS file, i.e. if its name
// matches one of the values in validGTFSFileNames.
func fileIsGTFSFile(fileName string) bool {
	for _, str := range validGTFSFileNames {
		if fileName == fmt.Sprintf("%s.txt", str) {
			return true
		}
	}

	return false
}

// Walks the fully extracted PTV GTFS zip and outputs each row of each GTFS CSV through a goroutine
// channel. Each row is wrapped in a GTFSRecord struct which contains the path of the parent file,
// the kind of file (stop_times, routes etc.), and the string slice of CSV data itself.
func walkPTVData(path string) chan GTFSRecord {
	c := make(chan GTFSRecord)
	var wg sync.WaitGroup

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatalf("Failure to access path %s: %s\n", path, err.Error())
		}

		// Check if we've arrived at a GTFS txt file.
		if !info.IsDir() && fileIsGTFSFile(info.Name()) {
			// Add a task to the waitgroup and fire off a goroutine.
			wg.Add(1)
			go func() {
				file, err := os.Open(path)
				if err != nil {
					log.Fatalf("Unable to open %s: %s\n", path, err.Error())
				}

				csvFile := csv.NewReader(file)
				// Skip the header row.
				csvFile.Read()
				// Iterate through the records of the current file.
				for {
					record, err := csvFile.Read()

					if err == io.EOF {
						break
					}

					if err != nil {
						log.Fatal(err)
					}

					recordType := strings.Split(info.Name(), ".")[0]
					c <- GTFSRecord{Path: path, Type: recordType, Contents: record}
				}
				wg.Done()
			}()
		}

		return err
	})

	if err != nil {
		log.Fatal(err)
	}

	// Close the channel after all records from all files have been read.
	go func() {
		wg.Wait()
		close(c)
	}()

	return c
}

// Extracts the .zip of the GTFS data supplied by PTV into a temporary directory, including
// subdirectories (1, 2, 3 etc.).
func extractPTVData(path string) error {
	log.Printf("Extracting %s...\n", path)
	// Extract the input zip.
	err := archiver.Unarchive(path, looseInputFiles)
	if err != nil {
		return err
	}
	log.Printf("Extracted %s. Walking...\n", path)

	// Walk the contents of the extracted input zip, and extract any inner zip files found.
	err = filepath.Walk(looseInputFiles, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatalf("Failure to access path %s: %s\n", path, err.Error())
		}

		// Check if we've hit an inner zip file.
		if info.Name() == innerZipFileName {
			// Extract zip to a directory of the same name in the same path.
			innerOutputPath := strings.Replace(path, ".zip", "", 1)

			log.Printf("Found %s file in path %s\n", innerZipFileName, path)
			err := archiver.Unarchive(path, innerOutputPath)
			if err != nil {
				log.Fatalf("Unable to unzip %s: %s\n", path, err.Error())
			}
			log.Printf("Extracted %s\n", path)
		}

		return nil
	})
	return err
}
