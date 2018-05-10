package checks

import (
	//"io/ioutil"
	"log"
	//"net/http"
	"os"
	//"strings"
	"testing"

	"github.com/globalsign/mgo"
	"github.com/pborman/uuid"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

var (
	defaultHealthAnnotations  = model.HealthAnnotations{EnableScrape: "true", Port: "8081"}
	diffPortHealthAnnotations = model.HealthAnnotations{EnableScrape: "true", Port: "8080"}
	noScrapeHealthAnnotations = model.HealthAnnotations{EnableScrape: "false", Port: "8081"}
)

const (
	dbURL = "localhost:27017"
)

type TestSuite struct {
	repo *db.MongoRepository
}

var s TestSuite

func TestMain(m *testing.M) {
	sess, err := mgo.Dial(dbURL)
	if err != nil {
		log.Fatalf("failed to create mongo session: %s", err.Error())
	}
	defer sess.Close()

	s.repo = db.NewMongoRepository(sess, uuid.New())

	code := m.Run()
	dbErr := s.repo.Session.DB(s.repo.DBName).DropDatabase()
	if dbErr != nil {
		log.Printf("Failed to drop database %v", s.repo.DBName)
	}
	os.Exit(code)
}

// func Test_DoHealthchecks(t *testing.T) {

// 	errs := make(chan error, 10)
// 	responses := make(chan model.HealthcheckResp, 10)
// 	healthchecks := make(chan model.Service, 10)

// 	expectedServiceHealthCheck := helpers.GenerateDummyCheck(helpers.String(10), helpers.String(10), "healthy")

// 	checker := healthChecker{client: &dummyClient{resp: http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("{\"description\":\"about endpoint response\"}"))}, err: nil}}

// }

// type dummyClient struct {
// 	resp http.Response
// 	err  error
// }

// func (d *dummyClient) Do(req *http.Request) (resp *http.Response, err error) {
// 	return &d.resp, d.err
// }
