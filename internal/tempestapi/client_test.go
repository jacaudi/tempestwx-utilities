package tempestapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// mockRoundTripper allows us to mock HTTP responses without running a server
type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// Helper to create a mock HTTP response
func mockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestNewClient(t *testing.T) {
	token := "test-token-123"
	client := NewClient(token)

	if client.token != token {
		t.Errorf("Expected token %s, got %s", token, client.token)
	}
}

// ============================================================================
// ListStations Tests
// ============================================================================

func TestListStations_Success_SingleStation(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "Test Station",
				"station_id": 12345,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 67890,
						"device_type": "ST",
						"serial_number": "ST-00012345"
					}
				]
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	// Replace http.DefaultClient temporarily
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(stations) != 1 {
		t.Fatalf("Expected 1 station, got %d", len(stations))
	}

	station := stations[0]
	if station.Name != "Test Station" {
		t.Errorf("Expected name 'Test Station', got %s", station.Name)
	}
	if station.StationID != 12345 {
		t.Errorf("Expected station ID 12345, got %d", station.StationID)
	}
	if station.deviceID != 67890 {
		t.Errorf("Expected device ID 67890, got %d", station.deviceID)
	}
	if station.serialNumber != "ST-00012345" {
		t.Errorf("Expected serial number 'ST-00012345', got %s", station.serialNumber)
	}

	expectedTime := time.Unix(1609459200, 0)
	if !station.CreatedAt.Equal(expectedTime) {
		t.Errorf("Expected created at %v, got %v", expectedTime, station.CreatedAt)
	}
}

func TestListStations_Success_MultipleStations(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "Station One",
				"station_id": 11111,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 10001,
						"device_type": "ST",
						"serial_number": "ST-00011111"
					}
				]
			},
			{
				"name": "Station Two",
				"station_id": 22222,
				"created_epoch": 1609545600,
				"devices": [
					{
						"device_id": 20002,
						"device_type": "ST",
						"serial_number": "ST-00022222"
					}
				]
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(stations) != 2 {
		t.Fatalf("Expected 2 stations, got %d", len(stations))
	}

	// Verify first station
	if stations[0].Name != "Station One" {
		t.Errorf("Expected first station name 'Station One', got %s", stations[0].Name)
	}
	if stations[0].StationID != 11111 {
		t.Errorf("Expected first station ID 11111, got %d", stations[0].StationID)
	}

	// Verify second station
	if stations[1].Name != "Station Two" {
		t.Errorf("Expected second station name 'Station Two', got %s", stations[1].Name)
	}
	if stations[1].StationID != 22222 {
		t.Errorf("Expected second station ID 22222, got %d", stations[1].StationID)
	}
}

func TestListStations_Success_MultipleDevicesPerStation(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "Mixed Device Station",
				"station_id": 12345,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 11111,
						"device_type": "HB",
						"serial_number": "HB-00011111"
					},
					{
						"device_id": 67890,
						"device_type": "ST",
						"serial_number": "ST-00012345"
					},
					{
						"device_id": 22222,
						"device_type": "AR",
						"serial_number": "AR-00022222"
					}
				]
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(stations) != 1 {
		t.Fatalf("Expected 1 station, got %d", len(stations))
	}

	// Should pick the ST device, not HB or AR
	station := stations[0]
	if station.deviceID != 67890 {
		t.Errorf("Expected device ID 67890 (ST device), got %d", station.deviceID)
	}
	if station.serialNumber != "ST-00012345" {
		t.Errorf("Expected serial number 'ST-00012345', got %s", station.serialNumber)
	}
}

func TestListStations_NoSTDevice(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "Hub Only Station",
				"station_id": 12345,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 11111,
						"device_type": "HB",
						"serial_number": "HB-00011111"
					}
				]
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should return empty slice when no ST devices found
	if len(stations) != 0 {
		t.Errorf("Expected 0 stations, got %d", len(stations))
	}
}

func TestListStations_EmptyStations(t *testing.T) {
	mockBody := `{
		"stations": [],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(stations) != 0 {
		t.Errorf("Expected 0 stations, got %d", len(stations))
	}
}

func TestListStations_MixedValidAndInvalid(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "No ST Device",
				"station_id": 11111,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 99999,
						"device_type": "HB",
						"serial_number": "HB-00099999"
					}
				]
			},
			{
				"name": "Valid Station",
				"station_id": 22222,
				"created_epoch": 1609545600,
				"devices": [
					{
						"device_id": 67890,
						"device_type": "ST",
						"serial_number": "ST-00022222"
					}
				]
			},
			{
				"name": "Empty Devices",
				"station_id": 33333,
				"created_epoch": 1609632000,
				"devices": []
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should only return the valid station
	if len(stations) != 1 {
		t.Fatalf("Expected 1 station, got %d", len(stations))
	}

	if stations[0].Name != "Valid Station" {
		t.Errorf("Expected 'Valid Station', got %s", stations[0].Name)
	}
}

func TestListStations_InvalidJSON(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, "invalid json{{{"),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	_, err := client.ListStations(context.Background())

	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestListStations_HTTPError(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		err: errors.New("network error"),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	_, err := client.ListStations(context.Background())

	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

func TestListStations_ContextCancellation(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		err: context.Canceled,
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.ListStations(ctx)

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

// ============================================================================
// GetObservations Tests
// ============================================================================

func TestGetObservations_Success(t *testing.T) {
	// Valid obs_st response with proper data structure
	mockBody := `{
		"type": "obs_st",
		"serial_number": "ORIGINAL-SERIAL",
		"hub_sn": "HB-00001234",
		"obs": [
			[1609459200, 0.5, 1.2, 2.3, 180, 3, 1013.25, 20.5, 65, 50000, 3, 500, 0, 0, 0, 0, 2.6, 1]
		],
		"firmware_revision": 143
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	station := Station{
		Name:         "Test Station",
		StationID:    12345,
		deviceID:     67890,
		serialNumber: "ST-00012345",
		CreatedAt:    time.Now(),
	}

	startTime := time.Unix(1609459000, 0)
	endTime := time.Unix(1609459300, 0)

	metrics, err := client.GetObservations(context.Background(), station, startTime, endTime)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) == 0 {
		t.Error("Expected metrics to be returned, got none")
	}

	// Verify that metrics were created (exact count depends on tempestudp.TempestObservationReport.Metrics())
	// We can't easily verify the serial number was set without accessing internal fields,
	// but the fact that metrics were created without error indicates success
}

func TestGetObservations_MultipleObservations(t *testing.T) {
	// Response with multiple observations
	mockBody := `{
		"type": "obs_st",
		"serial_number": "ORIGINAL-SERIAL",
		"hub_sn": "HB-00001234",
		"obs": [
			[1609459200, 0.5, 1.2, 2.3, 180, 3, 1013.25, 20.5, 65, 50000, 3, 500, 0, 0, 0, 0, 2.6, 1],
			[1609459260, 0.6, 1.3, 2.4, 185, 3, 1013.30, 20.6, 66, 51000, 3, 510, 0.1, 0, 0, 0, 2.6, 1],
			[1609459320, 0.7, 1.4, 2.5, 190, 3, 1013.35, 20.7, 67, 52000, 3, 520, 0.2, 0, 0, 0, 2.6, 1]
		],
		"firmware_revision": 143
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	station := Station{
		deviceID:     67890,
		serialNumber: "ST-00012345",
	}

	metrics, err := client.GetObservations(context.Background(), station, time.Now(), time.Now())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) == 0 {
		t.Error("Expected metrics from multiple observations, got none")
	}
}

func TestGetObservations_HTTPError(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		err: errors.New("network error"),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	station := Station{deviceID: 67890, serialNumber: "ST-00012345"}

	_, err := client.GetObservations(context.Background(), station, time.Now(), time.Now())

	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

func TestGetObservations_InvalidJSON(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, "not valid json"),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	station := Station{deviceID: 67890, serialNumber: "ST-00012345"}

	_, err := client.GetObservations(context.Background(), station, time.Now(), time.Now())

	if err == nil {
		t.Error("Expected JSON parsing error, got nil")
	}
}

// Note: We can't test the wrong report type case because the client uses log.Fatalf
// which exits the process. This would require refactoring the client to return an error instead.

func TestGetObservations_ContextCancellation(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		err: context.Canceled,
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	station := Station{deviceID: 67890, serialNumber: "ST-00012345"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.GetObservations(ctx, station, time.Now(), time.Now())

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

func TestGetObservations_EmptyObservations(t *testing.T) {
	// Valid response but with empty observations array
	mockBody := `{
		"type": "obs_st",
		"serial_number": "ST-00012345",
		"hub_sn": "HB-00001234",
		"obs": [],
		"firmware_revision": 143
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	station := Station{deviceID: 67890, serialNumber: "ST-00012345"}

	metrics, err := client.GetObservations(context.Background(), station, time.Now(), time.Now())

	if err != nil {
		t.Fatalf("Expected no error for empty observations, got %v", err)
	}

	// Empty observations should return empty metrics
	if len(metrics) != 0 {
		t.Errorf("Expected 0 metrics for empty observations, got %d", len(metrics))
	}
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestListStations_StationWithZeroDeviceID(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "Weird Station",
				"station_id": 12345,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 0,
						"device_type": "ST",
						"serial_number": "ST-00012345"
					}
				]
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// deviceId == 0 should be filtered out (line 76 check)
	if len(stations) != 0 {
		t.Errorf("Expected 0 stations (device ID is 0), got %d", len(stations))
	}
}

func TestListStations_StationWithEmptySerialNumber(t *testing.T) {
	mockBody := `{
		"stations": [
			{
				"name": "Missing Serial",
				"station_id": 12345,
				"created_epoch": 1609459200,
				"devices": [
					{
						"device_id": 67890,
						"device_type": "ST",
						"serial_number": ""
					}
				]
			}
		],
		"status": {
			"status_code": 0,
			"status_message": "SUCCESS"
		}
	}`

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{
		response: mockResponse(http.StatusOK, mockBody),
	}
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("test-token")
	stations, err := client.ListStations(context.Background())

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Empty serial number should be filtered out (line 76 check)
	if len(stations) != 0 {
		t.Errorf("Expected 0 stations (serial number is empty), got %d", len(stations))
	}
}

func TestGetObservations_URLParameters(t *testing.T) {
	// Test that URL is constructed correctly with all parameters
	var capturedURL string

	originalTransport := http.DefaultClient.Transport
	mockTransport := mockRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()

		mockResp := `{
			"type": "obs_st",
			"serial_number": "ORIG",
			"hub_sn": "HB-00001234",
			"obs": [[1609459200, 0.5, 1.2, 2.3, 180, 3, 1013.25, 20.5, 65, 50000, 3, 500, 0, 0, 0, 0, 2.6, 1]],
			"firmware_revision": 143
		}`

		return mockResponse(http.StatusOK, mockResp), nil
	})
	http.DefaultClient.Transport = mockTransport
	defer func() { http.DefaultClient.Transport = originalTransport }()

	client := NewClient("my-secret-token")
	station := Station{
		deviceID:     98765,
		serialNumber: "ST-00098765",
	}

	startTime := time.Unix(1609459000, 0)
	endTime := time.Unix(1609459300, 0)

	_, err := client.GetObservations(context.Background(), station, startTime, endTime)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify URL contains all required parameters
	if !strings.Contains(capturedURL, "device/98765") {
		t.Errorf("URL should contain device ID 98765: %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "token=my-secret-token") {
		t.Errorf("URL should contain token: %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "time_start=1609459000") {
		t.Errorf("URL should contain start time: %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "time_end=1609459300") {
		t.Errorf("URL should contain end time: %s", capturedURL)
	}
}

// mockRoundTripperFunc allows using a function as a RoundTripper
type mockRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f mockRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
