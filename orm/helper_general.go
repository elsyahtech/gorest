package orm

import "log"

func (orm *ORM) safeWriteString(s string) {
	if _, err := orm.StringBuilder.WriteString(s); err != nil {
		log.Printf("safeWriteString error: %v", err)
	}
}

func (orm *ORM) safeWriteByte(c byte) {
	if err := orm.StringBuilder.WriteByte(c); err != nil {
		log.Printf("safeWriteByte error: %v", err)
	}
}
