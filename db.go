package main

import (
	"encoding/binary"
	"fmt"
	bolt "go.etcd.io/bbolt"
)

const ( // Buckets
	counterEndingBucket   = "tick"
	counterInfiniteBucket = "infiniteBucket"
	roleBucket            = "roles"
	chatIDBucket          = "chatID"
	intervalBucket        = "interval"
	playersTodayBucket    = "playersToday"
)

const ( // Bullshit you need to work with bbolt/.txt
	lastChatIDKey  = "lastChatIDKey"
	userRWFileMode = 0600
	uint32Size     = 4
	uint64Size     = 8
	lastInterval   = "lastInterval"
	gamer          = "player"
	perm           = 0644
)

const ( // Important Users id
	daunitseID  = 841547487
	carpawellID = 209328250
)

const ( // Roles
	bomzRole = iota // не пользователь
	userRole        // пользователь, которому разрешено общаться с ботом
	adminRole
)

const ( // Bullshit you need to work with telegram
	privateCHat = "private"
	easterEgg   = "easter Egg"
	groupID     = -1002278052974
)

const ( // Ready to play status
	notReady = 0
	ready    = 1
)

type database struct {
	b *bolt.DB
}

func newDb(path string) (*database, error) {
	db, err := bolt.Open(path, userRWFileMode, bolt.DefaultOptions)
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}

	createBucketsFunc := func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(counterEndingBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(counterInfiniteBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(chatIDBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(intervalBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(roleBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(playersTodayBucket))
		return err
	}

	err = db.Update(createBucketsFunc)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("could not init database: %s", err)
	}

	return &database{
		b: db,
	}, nil
}

func (db *database) Close() error {
	return db.b.Close()
}

func (db *database) ResetBucket(bucket string) error {
	return db.b.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *database) GiveUserRoles(user int64, role byte) error {
	giveUserRole := func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(roleBucket))
		if b == nil {
			return fmt.Errorf("expected %s bucket does not exist", roleBucket)
		}
		userKey := make([]byte, uint64Size)
		binary.LittleEndian.PutUint64(userKey, uint64(user))
		val := []byte{role}

		return b.Put(userKey, val)
	}

	err := db.b.Update(giveUserRole)
	if err != nil {
		return fmt.Errorf("could not assign role: %w", err)
	}
	return nil
}

func (db *database) CheckUserRole(user int64) (byte, error) {
	var role byte

	userRole := func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(roleBucket))
		if b == nil {
			return fmt.Errorf("expected %s bucket does not exist", roleBucket)
		}
		userKey := make([]byte, uint64Size)
		binary.LittleEndian.PutUint64(userKey, uint64(user))
		val := b.Get(userKey)
		if val != nil {
			role = val[0]
		}
		return nil
	}

	err := db.b.View(userRole)
	if err != nil {
		return 0, fmt.Errorf("could not check user`s role: %w", err)
	}

	return role, nil
}

func (db *database) SaveLetsPlayStatus(user string, status uint32) error {
	err := db.b.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(playersTodayBucket))
		if b == nil {
			return fmt.Errorf("expected %s bucket does not exits", playersTodayBucket)
		}

		userKey := []byte(user)
		val := b.Get(userKey)

		val = make([]byte, uint32Size)
		binary.LittleEndian.PutUint32(val, status)

		return b.Put(userKey, val)
	})
	if err != nil {
		return fmt.Errorf("could not update val in db: %w", err)
	}

	return nil
}

type UserStatus struct {
	Name   string
	Status uint32
}

func (db *database) GetLetsPlayStatus() ([]UserStatus, error) {
	var stat []UserStatus
	getUsersStatus := func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(playersTodayBucket))
		if b == nil {
			return fmt.Errorf("expected %s bucket does not exist", playersTodayBucket)
		}
		return b.ForEach(func(k, v []byte) error {
			var us UserStatus
			us.Name = string(k)
			us.Status = binary.LittleEndian.Uint32(v)
			stat = append(stat, us)

			return nil
		})
	}
	err := db.b.View(getUsersStatus)
	if err != nil {
		return nil, fmt.Errorf("could not get all stat from db: %w", err)
	}

	return stat, nil
}
