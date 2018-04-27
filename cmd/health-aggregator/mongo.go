package main

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
	dbName  string
	session *mgo.Session
}

//NewMongoRepository connects to given mongo db providing abstraction layer from mgo driver
func NewMongoRepository(mongoSession *mgo.Session, dbName string) *MongoRepository {

	return &MongoRepository{
		session: mongoSession,
		dbName:  dbName,
	}
}

//WithNewSession create another copy of MongoServiceRequestRepo to be used on per single request base
func (m *MongoRepository) WithNewSession() *MongoRepository {

	return &MongoRepository{
		dbName:  m.dbName,
		session: m.session.Copy(),
	}
}

func (m *MongoRepository) db() *mgo.Database {
	return m.session.DB(m.dbName)
}

//Close associated mongo session copy with MongoServiceRequestRepo
func (m *MongoRepository) Close() {
	m.session.Close()
}
