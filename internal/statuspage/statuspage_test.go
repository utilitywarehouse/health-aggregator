package statuspage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

var (
	baseURL            string
	apiStub            *httptest.Server
	componentID        = "comp1"
	statuspageIOPageID = "abc"
	statuspageIOAPIKey = "123"
)

func Test_MapUWStatusToStatuspageIOStatus(t *testing.T) {

	var cases = []struct {
		name          string
		uwStatus      string
		expStatus     string
		expError      bool
		expErrContent string
	}{
		{"UW status healthy", constants.Healthy, operational, false, ""},
		{"UW status unhealthy", constants.Unhealthy, partialOutage, false, ""},
		{"UW status degraded", constants.Degraded, degradedPerformance, false, ""},
		{"UW status something unknown", "something-unknown", "", true, "unknown"},
		{"No UW status provided - empty string", "", "", true, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			returnedStatus, err := MapUWStatusToStatuspageIOStatus(tc.uwStatus)

			assert.Equal(t, tc.expStatus, returnedStatus)
			if tc.expError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expErrContent)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expStatus, returnedStatus)
			}
		})
	}
}
func Test_SetComponentStatus(t *testing.T) {

	setupServerReturn200()

	statusPageUpdater := Updater{StatusPageID: statuspageIOPageID, Client: client, APIKey: statuspageIOAPIKey, StatusPageBaseURL: apiStub.URL}

	component := model.Component{ID: componentID, Status: operational}

	err := statusPageUpdater.SetComponentStatus(component)
	require.NoError(t, err)

	setupServerReturn4XX()

	statusPageUpdater.StatusPageBaseURL = apiStub.URL

	err = statusPageUpdater.SetComponentStatus(component)
	assert.Contains(t, err.Error(), "received non-200 status code")
}

func setupServerReturn200() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func setupServerReturn4XX() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
}
