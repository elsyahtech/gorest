package orm

func (orm *ORM) setMongoWriteError(operation string, err error) error {
	return orm.setDatabaseOperationError(operation, err)
}
