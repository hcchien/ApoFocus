package ingest

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

func readMetadata(path string, fallbackTime time.Time) (Inspection, error) {
	file, err := os.Open(path)
	if err != nil {
		return Inspection{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{
		Filename:      filepath.Base(path),
		Title:         strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		TakenAt:       fallbackTime,
		Year:          fallbackTime.Year(),
		FileType:      strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")),
		FileSizeBytes: stat.Size(),
		Metadata:      map[string]any{},
		AspectRatio:   "landscape",
	}

	if config, _, err := image.DecodeConfig(file); err == nil {
		result.Dimensions = fmt.Sprintf("%d × %d", config.Width, config.Height)
		if config.Height > config.Width {
			result.AspectRatio = "portrait"
		}
		result.Metadata["width"] = config.Width
		result.Metadata["height"] = config.Height
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Inspection{}, err
	}
	x, err := exif.Decode(file)
	if err != nil && (x == nil || exif.IsCriticalError(err)) {
		return result, nil
	}
	if takenAt, err := x.DateTime(); err == nil {
		result.TakenAt = takenAt
		result.Year = takenAt.Year()
	}
	result.Camera = exifString(x, exif.Model)
	result.Lens = exifString(x, exif.LensModel)
	result.Aperture = exifRational(x, exif.FNumber, "f/", 1)
	result.ShutterSpeed = exifExposure(x)
	result.FocalLength = exifRational(x, exif.FocalLength, "", 0)
	if result.FocalLength != "" {
		result.FocalLength += "mm"
	}
	if tag, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if value, err := tag.Int(0); err == nil {
			result.ISO = value
		}
	}
	if lat, long, err := x.LatLong(); err == nil {
		result.Location = &Location{Latitude: lat, Longitude: long}
	}
	result.Metadata["exifParsed"] = true
	return result, nil
}

func exifString(x *exif.Exif, field exif.FieldName) string {
	tag, err := x.Get(field)
	if err != nil {
		return ""
	}
	value, err := tag.StringVal()
	if err == nil {
		return strings.TrimSpace(value)
	}
	return strings.Trim(tag.String(), `" `)
}

func exifRational(x *exif.Exif, field exif.FieldName, prefix string, precision int) string {
	tag, err := x.Get(field)
	if err != nil {
		return ""
	}
	value, err := tag.Rat(0)
	if err != nil {
		return ""
	}
	number, _ := value.Float64()
	return prefix + strconv.FormatFloat(number, 'f', precision, 64)
}

func exifExposure(x *exif.Exif) string {
	tag, err := x.Get(exif.ExposureTime)
	if err != nil {
		return ""
	}
	value, err := tag.Rat(0)
	if err != nil {
		return ""
	}
	if value.Num().Int64() == 1 {
		return fmt.Sprintf("1/%ds", value.Denom().Int64())
	}
	number, _ := value.Float64()
	return strconv.FormatFloat(number, 'f', 2, 64) + "s"
}
