package db

import (
	"github.com/globalsign/mgo"
)

//MongoConnector provides way interact with mongo connection
type MongoConnector interface {
	Close()
	WithNewSession() *MongoRepository
}

// MongoRepository was a active session to connect to mongo DB
type MongoRepository struct {
	DBName  string
	Session *mgo.Session
}

//NewMongoRepository connects to given mongo db providing abstraction layer from mgo driver
func NewMongoRepository(mongoSession *mgo.Session, dbName string) *MongoRepository {

	return &MongoRepository{
		Session: mongoSession,
		DBName:  dbName,
	}
}

//WithNewSession create another copy of MongoServiceRequestRepo to be used on per single request base
func (m *MongoRepository) WithNewSession() *MongoRepository {

	return &MongoRepository{
		DBName:  m.DBName,
		Session: m.Session.Copy(),
	}
}

// Db returns the Database
func (m *MongoRepository) Db() *mgo.Database {
	return m.Session.DB(m.DBName)
}

//Close associated mongo session copy with MongoServiceRequestRepo
func (m *MongoRepository) Close() {
	m.Session.Close()
}
