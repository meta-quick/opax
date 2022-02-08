package topdown

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cockroachdb/pebble"
	"github.com/open-policy-agent/opa/ast"
	"strconv"
	"time"
)

type persistApi interface {
  Set(key string, value interface{}) error;
  SetString(key string, value string) error;
  SetInteger(key string, value int64) error;
  SetFloat(key string, value float64) error;
  SetBool(key string, value bool) error;
  SetBytes(key string, value []byte) error;
  Get(key string) (interface{},error);
  GetString(key string) (string,error);
  GetInteger(key string) (int64,error);
  GetBool(key string) (bool,error);
  GetFloat(key string) (float64,error);
  GetBytes(key string) ([]byte,error);
  Delete(key string) error;
}

type pebbleStorage struct {
  db *pebble.DB;
}

func NewPebbleStorage(path string, options pebble.Options) persistApi {
  db, err := pebble.Open(path, &options)
  if err != nil {
	panic(err)
  }
  return &pebbleStorage{db: db}
}

func (this *pebbleStorage ) Set(key string, value interface{}) error {
   switch v := value.(type) {
   case string:
	   val := []byte(v)
	   this.db.Set([]byte(key), val,nil);
   case []byte:
	   this.db.Set([]byte(key), v,nil);
   case int:
	   this.db.Set([]byte(key), []byte(fmt.Sprintf("%d", v)),nil);
   case float64:
	   this.db.Set([]byte(key), []byte( fmt.Sprintf("%v", v)),nil);
   case bool:
	   this.db.Set([]byte(key), []byte(fmt.Sprintf("%t", v)),nil);
   case nil:
	   this.db.Set([]byte(key), []byte(""),nil);
   default:
	   println("Unsupported type",v);
	   return errors.New("Unsupported type");
   }
   return nil;
}

func (this *pebbleStorage ) SetString(key string, value string) error {
	return this.Set(key, value);
}

func (this *pebbleStorage ) SetInteger(key string, value int64) error {
	return this.Set(key, value);
}

func (this *pebbleStorage ) SetFloat(key string, value float64) error {
	return this.Set(key, value);
}

func (this *pebbleStorage ) SetBool(key string, value bool) error {
	return this.Set(key, value);
}

func (this *pebbleStorage ) SetBytes(key string, value []byte) error {
	return this.Set(key, value);
}

func (this *pebbleStorage ) Get(key string) (interface{},error) {
  value, _, err := this.db.Get([]byte(key))
  return value, err
}

func (this *pebbleStorage ) GetString(key string) (string,error) {
	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	return string(value),nil
}


func (this *pebbleStorage ) GetInteger(key string) (int64,error) {
	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	return strconv.ParseInt(string(value), 10, 64)
}

func (this *pebbleStorage ) GetBool(key string) (bool, error) {
	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	return strconv.ParseBool(string(value))
}

func (this *pebbleStorage ) GetFloat(key string) (float64,error) {
	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	return strconv.ParseFloat(string(value), 64)
}

func (this *pebbleStorage ) GetBytes(key string) ([]byte,error) {
	value, _, err := this.db.Get([]byte(key))
	return value,err
}


func (this *pebbleStorage ) Delete(key string)  error {
  return this.db.Delete([]byte(key),nil)
}

type Counter struct {
	Value int64;
	Duration int64;
	Timestamp map[int64]int64;
}

func NewCounter(duration int64) Counter {
	return Counter{0,duration,make(map[int64]int64)}
}

func (this *Counter) GetValue() int64 {
	now := time.Now().Unix();
	this.Value = 0;
	for k, v := range this.Timestamp {
		if  now - k < this.Duration {
			this.Value += v
		}
	}
	return this.Value;
}

func (this *Counter) Add(val int64) {
	now := time.Now().Unix();
	for k, _ := range this.Timestamp {
		if now - k > this.Duration {
			delete(this.Timestamp, k)
		}
	}
	key := time.Now().Unix()
	if _, ok := this.Timestamp[key]; ok {
		this.Timestamp[key] += val;
	} else {
		this.Timestamp[key] = val;
	}
}

func (this *Counter) Inc() {
	this.Add(1);
}

func (this *Counter) Dec() {
	this.Add(-1);
}

func (this *Counter) Reset() {
	this.Timestamp = make(map[int64]int64);
	this.Value = 0
}

func (this *Counter) GetDuration() int64{
	return this.Duration;
}

func CounterAdd(key,value,duration ast.Value) ( output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	lvalue, ok2 := value.(ast.Number)
	lduration, ok3 := duration.(ast.Number)

	if ok1 && ok2 && ok3 {
		var counter Counter;
		if value, err := store.GetBytes(lkey.String()) ; err == nil {
		    lduration, _ :=	lduration.Int64()
			counter = NewCounter(lduration)
			json.Unmarshal(value,&counter)
		} else {
			lduration, _ :=	lduration.Int64()
			counter = NewCounter(lduration)
		}
		lvalue, _ := lvalue.Int64()
		counter.Add(lvalue)
		value, _ := json.Marshal(counter)
		store.SetBytes(lkey.String(),value)
		output = ast.Number(fmt.Sprintf("%d", counter.GetValue()))
	} else {
		err = errors.New("Invalid input type")
		output = ast.Number("0")
	}
	return
}

func CounterDelete(key ast.Value) (output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	if ok1 {
		if err = store.Delete(lkey.String()); err == nil {
			return ast.Boolean(true), err
		}
	}

	return ast.Boolean(false), err
}

var store = NewPebbleStorage("/tmp/test.db",pebble.Options{});
func init(){
	//RegisterFunctionalBuiltin2("Counter.Get", CounterGet)
	RegisterFunctionalBuiltin1(ast.TimedCounterDelete.Name, CounterDelete)
	RegisterFunctionalBuiltin3(ast.TimedCounterAdd.Name, CounterAdd)
}