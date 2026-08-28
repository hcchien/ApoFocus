package catalog

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

var ErrNotFound = errors.New("photo not found")
var ErrConflict = errors.New("catalog revision conflict")

type MemoryStore struct {
	photos []Photo
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{photos: demoPhotos()}
}

func (s *MemoryStore) List(_ context.Context, filter Filter) (PhotoPage, error) {
	filtered := make([]Photo, 0, len(s.photos))
	for _, photo := range s.photos {
		if matches(photo, filter) {
			filtered = append(filtered, photo)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].TakenAt.After(filtered[j].TakenAt) })
	total := len(filtered)
	start := min(filter.Offset, total)
	end := min(start+filter.Limit, total)
	return PhotoPage{Items: filtered[start:end], Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Photo, error) {
	for _, photo := range s.photos {
		if photo.ID == id {
			return photo, nil
		}
	}
	return Photo{}, ErrNotFound
}

func (s *MemoryStore) Facets(_ context.Context) (Facets, error) {
	years, projects, tags := map[string]int{}, map[string]int{}, map[string]int{}
	cameras, lenses := map[string]int{}, map[string]int{}
	for _, photo := range s.photos {
		years[itoa(photo.Year)]++
		projects[photo.Project]++
		cameras[photo.Camera]++
		lenses[photo.Lens]++
		for _, tag := range photo.Tags {
			tags[tag]++
		}
	}
	return Facets{
		Years: facetCounts(years, true), Projects: facetCounts(projects, false),
		Tags: facetCounts(tags, false), Cameras: facetCounts(cameras, false), Lenses: facetCounts(lenses, false),
	}, nil
}

func (s *MemoryStore) Similar(_ context.Context, id string, limit int) ([]SimilarPhoto, error) {
	anchor, err := s.Get(context.Background(), id)
	if err != nil {
		return nil, err
	}
	result := make([]SimilarPhoto, 0, len(s.photos)-1)
	for _, photo := range s.photos {
		if photo.ID == id {
			continue
		}
		result = append(result, SimilarPhoto{Photo: photo, Similarity: cosine(anchor.Embedding, photo.Embedding)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Similarity > result[j].Similarity })
	return result[:min(limit, len(result))], nil
}

func (s *MemoryStore) Update(_ context.Context, id string, update PhotoUpdate) (Photo, error) {
	for index := range s.photos {
		photo := &s.photos[index]
		if photo.ID != id {
			continue
		}
		if update.Revision == nil || *update.Revision != photo.Revision {
			return Photo{}, ErrConflict
		}
		if update.Title != nil {
			photo.Title = strings.TrimSpace(*update.Title)
		}
		if update.Project != nil {
			photo.Project = strings.TrimSpace(*update.Project)
		}
		if update.TakenAt != nil {
			photo.TakenAt = *update.TakenAt
			photo.Year = update.TakenAt.Year()
		}
		if update.Tags != nil {
			photo.Tags = append([]string(nil), (*update.Tags)...)
		}
		if update.Camera != nil {
			photo.Camera = strings.TrimSpace(*update.Camera)
		}
		if update.Lens != nil {
			photo.Lens = strings.TrimSpace(*update.Lens)
		}
		if update.Description != nil {
			photo.Description = *update.Description
		}
		if update.Copyright != nil {
			photo.Copyright = *update.Copyright
		}
		if update.Rating != nil {
			photo.Rating = *update.Rating
		}
		if update.Favorite != nil {
			photo.Favorite = *update.Favorite
		}
		photo.Revision++
		return *photo, nil
	}
	return Photo{}, ErrNotFound
}

func matches(photo Photo, filter Filter) bool {
	if filter.Year != 0 && photo.Year != filter.Year || filter.Project != "" && photo.Project != filter.Project {
		return false
	}
	if filter.Camera != "" && photo.Camera != filter.Camera || filter.Lens != "" && photo.Lens != filter.Lens {
		return false
	}
	if filter.MinISO != 0 && photo.ISO < filter.MinISO || filter.MaxISO != 0 && photo.ISO > filter.MaxISO {
		return false
	}
	if filter.HasLocation != nil && (*filter.HasLocation != (photo.Location != nil)) {
		return false
	}
	for _, wanted := range filter.Tags {
		if !containsFold(photo.Tags, wanted) {
			return false
		}
	}
	if query := strings.ToLower(strings.TrimSpace(filter.Query)); query != "" {
		haystack := strings.ToLower(strings.Join(append([]string{photo.Title, photo.Project, photo.Camera, photo.Lens}, photo.Tags...), " "))
		if !strings.Contains(haystack, query) {
			return false
		}
	}
	return true
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func cosine(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func facetCounts(values map[string]int, descending bool) []FacetCount {
	result := make([]FacetCount, 0, len(values))
	for value, count := range values {
		if value != "" {
			result = append(result, FacetCount{Value: value, Count: count})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if descending {
			return result[i].Value > result[j].Value
		}
		if result[i].Count == result[j].Count {
			return result[i].Value < result[j].Value
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func demoPhotos() []Photo {
	date := func(value string) time.Time { parsed, _ := time.Parse(time.RFC3339, value); return parsed }
	loc := func(name string, lat, lng float64) *Location {
		return &Location{Name: name, Latitude: lat, Longitude: lng}
	}
	img := func(id string, width int) string {
		return "https://images.unsplash.com/" + id + "?auto=format&fit=crop&w=" + itoa(width) + "&q=86"
	}
	return []Photo{
		{ID: "a1", Title: "雨季的白牆", Year: 2026, Project: "島嶼日常", TakenAt: date("2026-07-18T14:32:00+08:00"), Tags: []string{"街頭", "雨天", "人物"}, Camera: "Leica Q3", Lens: "Summilux 28mm f/1.7", Aperture: "f/2.8", ShutterSpeed: "1/500s", ISO: 400, FocalLength: "28mm", Dimensions: "9520 × 6336", FileType: "DNG", FileSize: "84.2 MB", Location: loc("台北・大同區", 25.063, 121.513), ImageURL: img("photo-1519501025264-65ba15a82390", 1800), ThumbnailURL: img("photo-1519501025264-65ba15a82390", 900), AspectRatio: "portrait", Dominant: "#77807a", Embedding: []float32{.92, .78, .16, .11, .44, .22, .09, .38}},
		{ID: "a2", Title: "沿海公路 No. 12", Year: 2026, Project: "潮線", TakenAt: date("2026-06-02T17:48:00+08:00"), Tags: []string{"海岸", "公路", "黃昏"}, Camera: "Sony α1 II", Lens: "FE 35mm F1.4 GM", Aperture: "f/5.6", ShutterSpeed: "1/320s", ISO: 100, FocalLength: "35mm", Dimensions: "8640 × 5760", FileType: "ARW", FileSize: "71.8 MB", Location: loc("花蓮・豐濱", 23.602, 121.522), ImageURL: img("photo-1470770841072-f978cf4d019e", 1800), ThumbnailURL: img("photo-1470770841072-f978cf4d019e", 900), AspectRatio: "landscape", Dominant: "#7c8f91", Embedding: []float32{.12, .31, .94, .72, .17, .48, .66, .20}},
		{ID: "a3", Title: "凌晨四點的市場", Year: 2025, Project: "島嶼日常", TakenAt: date("2025-11-23T04:12:00+08:00"), Tags: []string{"市場", "夜間", "紀實"}, Camera: "Fujifilm GFX100 II", Lens: "GF 55mm F1.7", Aperture: "f/2", ShutterSpeed: "1/125s", ISO: 1600, FocalLength: "55mm", Dimensions: "11648 × 8736", FileType: "RAF", FileSize: "198 MB", Location: loc("基隆・仁愛區", 25.128, 121.741), ImageURL: img("photo-1519501025264-65ba15a82390", 1800), ThumbnailURL: img("photo-1519501025264-65ba15a82390", 900), AspectRatio: "landscape", Dominant: "#343b40", Embedding: []float32{.81, .69, .19, .14, .51, .25, .10, .42}},
		{ID: "a4", Title: "山霧緩慢上升", Year: 2025, Project: "邊界地景", TakenAt: date("2025-09-12T06:21:00+08:00"), Tags: []string{"山林", "霧", "清晨"}, Camera: "Hasselblad X2D 100C", Lens: "XCD 38V", Aperture: "f/8", ShutterSpeed: "1/60s", ISO: 200, FocalLength: "38mm", Dimensions: "11656 × 8742", FileType: "3FR", FileSize: "212 MB", Location: loc("南投・仁愛", 24.023, 121.131), ImageURL: img("photo-1464822759023-fed622ff2c3b", 1800), ThumbnailURL: img("photo-1464822759023-fed622ff2c3b", 900), AspectRatio: "landscape", Dominant: "#53635d", Embedding: []float32{.10, .27, .91, .81, .13, .55, .73, .18}},
		{ID: "a5", Title: "午休時間", Year: 2025, Project: "城市切片", TakenAt: date("2025-08-03T12:08:00+08:00"), Tags: []string{"建築", "光影", "極簡"}, Camera: "Leica Q3", Lens: "Summilux 28mm f/1.7", Aperture: "f/8", ShutterSpeed: "1/1000s", ISO: 100, FocalLength: "28mm", Dimensions: "9520 × 6336", FileType: "DNG", FileSize: "79.4 MB", ImageURL: img("photo-1487958449943-2429e8be8625", 1800), ThumbnailURL: img("photo-1487958449943-2429e8be8625", 900), AspectRatio: "portrait", Dominant: "#b9aa92", Embedding: []float32{.66, .31, .28, .19, .80, .11, .06, .45}},
		{ID: "a6", Title: "她與窗邊的植物", Year: 2024, Project: "房間裡的人", TakenAt: date("2024-12-17T15:42:00+08:00"), Tags: []string{"肖像", "室內", "自然光"}, Camera: "Canon EOS R5 Mark II", Lens: "RF 50mm F1.2 L", Aperture: "f/1.8", ShutterSpeed: "1/250s", ISO: 320, FocalLength: "50mm", Dimensions: "8192 × 5464", FileType: "CR3", FileSize: "61.3 MB", ImageURL: img("photo-1500648767791-00dcc994a43e", 1800), ThumbnailURL: img("photo-1500648767791-00dcc994a43e", 900), AspectRatio: "portrait", Dominant: "#8a765f", Embedding: []float32{.85, .88, .10, .08, .39, .29, .14, .36}},
		{ID: "a7", Title: "鹽田最後一道光", Year: 2024, Project: "潮線", TakenAt: date("2024-10-29T17:31:00+08:00"), Tags: []string{"地景", "夕陽", "水面"}, Camera: "Sony α1 II", Lens: "FE 24-70mm F2.8 GM II", Aperture: "f/11", ShutterSpeed: "1/80s", ISO: 100, FocalLength: "42mm", Dimensions: "8640 × 5760", FileType: "ARW", FileSize: "68.1 MB", Location: loc("台南・七股", 23.142, 120.101), ImageURL: img("photo-1500530855697-b586d89ba3ee", 1800), ThumbnailURL: img("photo-1500530855697-b586d89ba3ee", 900), AspectRatio: "landscape", Dominant: "#b78660", Embedding: []float32{.14, .28, .88, .74, .18, .50, .62, .29}},
		{ID: "a8", Title: "回家的末班車", Year: 2024, Project: "城市切片", TakenAt: date("2024-03-08T23:16:00+08:00"), Tags: []string{"交通", "夜間", "霓虹"}, Camera: "Fujifilm X-Pro3", Lens: "XF 23mm F1.4", Aperture: "f/2", ShutterSpeed: "1/60s", ISO: 3200, FocalLength: "23mm", Dimensions: "6240 × 4160", FileType: "RAF", FileSize: "56.7 MB", Location: loc("新北・板橋", 25.015, 121.464), ImageURL: img("photo-1519608487953-e999c86e7455", 1800), ThumbnailURL: img("photo-1519608487953-e999c86e7455", 900), AspectRatio: "landscape", Dominant: "#383a48", Embedding: []float32{.72, .55, .24, .19, .62, .18, .21, .49}},
		{ID: "a9", Title: "泳池邊的午後", Year: 2023, Project: "房間裡的人", TakenAt: date("2023-08-19T14:09:00+08:00"), Tags: []string{"肖像", "夏日", "色彩"}, Camera: "Canon EOS R5 Mark II", Lens: "RF 85mm F1.2 L", Aperture: "f/2.2", ShutterSpeed: "1/1600s", ISO: 100, FocalLength: "85mm", Dimensions: "8192 × 5464", FileType: "CR3", FileSize: "64.8 MB", ImageURL: img("photo-1502685104226-ee32379fefbe", 1800), ThumbnailURL: img("photo-1502685104226-ee32379fefbe", 900), AspectRatio: "portrait", Dominant: "#5a8790", Embedding: []float32{.82, .91, .08, .06, .42, .31, .16, .33}},
		{ID: "a10", Title: "北方的無人車站", Year: 2023, Project: "邊界地景", TakenAt: date("2023-02-14T09:44:00+08:00"), Tags: []string{"鐵道", "冬季", "建築"}, Camera: "Hasselblad X2D 100C", Lens: "XCD 55V", Aperture: "f/7.1", ShutterSpeed: "1/250s", ISO: 200, FocalLength: "55mm", Dimensions: "11656 × 8742", FileType: "3FR", FileSize: "206 MB", Location: loc("北海道・美瑛", 43.589, 142.467), ImageURL: img("photo-1470770841072-f978cf4d019e", 1800), ThumbnailURL: img("photo-1470770841072-f978cf4d019e", 900), AspectRatio: "landscape", Dominant: "#a9b0ae", Embedding: []float32{.18, .25, .84, .77, .26, .45, .70, .15}},
		{ID: "a11", Title: "練習曲 No. 3", Year: 2022, Project: "房間裡的人", TakenAt: date("2022-11-06T10:30:00+08:00"), Tags: []string{"肖像", "黑白", "室內"}, Camera: "Leica M11 Monochrom", Lens: "Summilux-M 50mm f/1.4", Aperture: "f/2", ShutterSpeed: "1/125s", ISO: 800, FocalLength: "50mm", Dimensions: "9528 × 6328", FileType: "DNG", FileSize: "92.1 MB", ImageURL: img("photo-1517841905240-472988babdf9", 1800), ThumbnailURL: img("photo-1517841905240-472988babdf9", 900), AspectRatio: "portrait", Dominant: "#656565", Embedding: []float32{.78, .86, .12, .09, .36, .35, .11, .41}},
		{ID: "a12", Title: "島嶼南端", Year: 2022, Project: "邊界地景", TakenAt: date("2022-05-21T15:53:00+08:00"), Tags: []string{"海岸", "岩石", "風景"}, Camera: "Sony α1 II", Lens: "FE 16-35mm F2.8 GM II", Aperture: "f/9", ShutterSpeed: "1/400s", ISO: 100, FocalLength: "21mm", Dimensions: "8640 × 5760", FileType: "ARW", FileSize: "70.2 MB", Location: loc("屏東・恆春", 21.901, 120.852), ImageURL: img("photo-1500530855697-b586d89ba3ee", 1800), ThumbnailURL: img("photo-1500530855697-b586d89ba3ee", 900), AspectRatio: "landscape", Dominant: "#68858c", Embedding: []float32{.09, .30, .96, .69, .15, .54, .61, .25}},
	}
}
