package orm

func (orm *ORM) setError(message string, err error) error {
	orm.Message = message
	orm.Error = err

	return orm.Error
}
