package orm

func (orm *ORM) setScyllaWriteError(operation string, err error) error {
	return orm.setDatabaseOperationError(operation, err)
}
