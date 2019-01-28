package main

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var extractedInputPath = "./gtfs_in"
var extractedOutputPath = "./gtfs_out"
var innerZipFileName = "google_transit.zip"
var validGTFSFileNames = []string{"agency", "calendar_dates", "calendar", "routes", "stop_times", "stops", "trips"}

var outputData = map[string][][]string{
	"agency":         [][]string{{"agency_id", "agency_name", "agency_url", "agency_timezone", "agency_lang"}},
	"calendar_dates": [][]string{{"service_id", "date", "exception_type"}},
	"calendar":       [][]string{{"service_id", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "start_date", "end_date"}},
	"routes":         [][]string{{"route_id", "agency_id", "route_short_name", "route_long_name", "route_type", "route_color", "route_text_color"}},
	"stop_times":     [][]string{{"trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence", "stop_headsign", "pickup_type", "drop_off_type", "shape_dist_traveled"}},
	"stops":          [][]string{{"stop_id", "stop_name", "stop_lat", "stop_lon"}},
	"trips":          [][]string{{"route_id", "service_id", "trip_id", "shape_id", "trip_headsign", "direction_id"}},
}

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

	for record := range walkPTVData(extractedInputPath) {
		if !isGTFSRecordExisting(record, outputData[record.Type]) {
			outputData[record.Type] = append(outputData[record.Type], record.Contents)
		}
	}

	writeOutput()

	fmt.Println(outputData["agency"])

	//err = cleanup()

	//if err != nil {
	//	log.Fatal(err)
	//}

}

func cleanup() error {
	err := os.RemoveAll(extractedInputPath)
	return err
}

func writeOutput() {
	if _, err := os.Stat(extractedOutputPath); os.IsNotExist(err) {
		os.MkdirAll(extractedOutputPath, os.ModePerm)
	}

	for k, v := range outputData {
		writeCSV(v, fmt.Sprintf("%s/%s.txt", extractedOutputPath, k))
	}
}

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
	_, err := Unzip(path, extractedInputPath)
	if err != nil {
		return err
	}
	log.Printf("Extracted %s. Walking...\n", path)

	// Walk the contents of the extracted input zip, and extract any inner zip files found.
	err = filepath.Walk(extractedInputPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatalf("Failure to access path %s: %s\n", path, err.Error())
		}

		// Check if we've hit an inner zip file.
		if info.Name() == innerZipFileName {
			// Extract zip to a directory of the same name in the same path.
			innerOutputPath := strings.Replace(path, ".zip", "", 1)

			log.Printf("Found %s file in path %s\n", innerZipFileName, path)
			_, err := Unzip(path, innerOutputPath)
			if err != nil {
				log.Fatalf("Unable to unzip %s: %s\n", path, err.Error())
			}
			log.Printf("Extracted %s\n", path)
		}

		return nil
	})
	return err
}

// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
func Unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}
		defer rc.Close()

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {

			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)

		} else {

			// Make File
			if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
				return filenames, err
			}

			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return filenames, err
			}

			_, err = io.Copy(outFile, rc)

			// Close the file without defer to close before next iteration of loop
			outFile.Close()

			if err != nil {
				return filenames, err
			}

		}
	}
	return filenames, nil
}
