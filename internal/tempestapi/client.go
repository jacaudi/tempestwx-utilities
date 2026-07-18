package tempestapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"tempestwx-utilities/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
)

type Client struct {
	token string
	http  *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

type Station struct {
	Name         string
	StationID    int
	deviceID     int
	serialNumber string
	CreatedAt    time.Time
}

func (c *Client) ListStations(ctx context.Context) ([]Station, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://swd.weatherflow.com/swd/rest/stations", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weatherflow API status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	var data struct {
		Stations []struct {
			CreatedEpoch int64 `json:"created_epoch"`
			Devices      []struct {
				DeviceID     int    `json:"device_id"`
				DeviceType   string `json:"device_type"`
				SerialNumber string `json:"serial_number"`
			} `json:"devices"`
			Name      string `json:"name"`
			StationID int    `json:"station_id"`
		} `json:"stations"`
		Status struct {
			StatusCode    int    `json:"status_code"`
			StatusMessage string `json:"status_message"`
		} `json:"status"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.Status.StatusCode != 0 {
		return nil, fmt.Errorf("weatherflow status_code %d: %s", data.Status.StatusCode, data.Status.StatusMessage)
	}

	var out []Station
	for _, station := range data.Stations {
		var deviceId int
		var instance string
		for _, dev := range station.Devices {
			if dev.DeviceType == "ST" {
				deviceId = dev.DeviceID
				instance = dev.SerialNumber
			}
		}

		if deviceId != 0 && instance != "" {
			out = append(out, Station{
				Name:         station.Name,
				deviceID:     deviceId,
				serialNumber: instance,
				StationID:    station.StationID,
				CreatedAt:    time.Unix(station.CreatedEpoch, 0),
			})
		}
	}
	return out, nil
}

func (c *Client) GetObservations(ctx context.Context, station Station, startAt time.Time, endAt time.Time) ([]prometheus.Metric, error) {
	url := fmt.Sprintf("https://swd.weatherflow.com/swd/rest/observations/device/%d?time_start=%d&time_end=%d", station.deviceID, startAt.Unix(), endAt.Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weatherflow API status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	report, err := tempestudp.ParseReport(body)
	if err != nil {
		log.Printf("read %s", string(body))
		return nil, err
	}

	switch r := report.(type) {
	case *tempestudp.TempestObservationReport:
		r.SerialNumber = station.serialNumber
	default:
		return nil, fmt.Errorf("unhandled report type %T", report)
	}

	metrics := report.Metrics()
	return metrics, nil
}
