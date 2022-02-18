package topdown

import (
	"errors"
	"fmt"
	//"encoding/json"
	"github.com/bytedance/sonic"
	"github.com/cockroachdb/pebble"
	"github.com/meta-quick/opa/ast"
	"strconv"
	"time"
)

type PersistApi interface {
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
  cache map[string][]byte;
}

func NewPebbleStorage(path string, options pebble.Options) PersistApi {
  db, err := pebble.Open(path, &options)
  if err != nil {
	panic(err)
  }
  _cache := make(map[string][]byte)
  return &pebbleStorage{db: db,cache: _cache}
}

func (this *pebbleStorage ) Set(key string, value interface{}) error {
   switch v := value.(type) {
   case string:
	   val := []byte(v)
	   this.db.Set([]byte(key), val,nil);
	   this.cache[key] = val;
   case []byte:
	   this.db.Set([]byte(key), v,nil);
	   this.cache[key] = v;
   case int:
	   this.db.Set([]byte(key), []byte(fmt.Sprintf("%d", v)),nil);
	   this.cache[key] = []byte(fmt.Sprintf("%d", v));
   case int64:
	   this.db.Set([]byte(key), []byte(fmt.Sprintf("%d", v)),nil);
	   this.cache[key] = []byte(fmt.Sprintf("%d", v));
   case float64:
	   this.db.Set([]byte(key), []byte( fmt.Sprintf("%v", v)),nil);
	   this.cache[key] = []byte(fmt.Sprintf("%v", v));
   case bool:
	   this.db.Set([]byte(key), []byte(fmt.Sprintf("%t", v)),nil);
	   this.cache[key] = []byte(fmt.Sprintf("%t", v));
   case nil:
	   this.db.Set([]byte(key), []byte(""),nil);
	   this.cache[key] = []byte("");
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
	if val,ok := this.cache[key]; ok {
		return val,nil;
	}
    value, _, err := this.db.Get([]byte(key))
	this.cache[key] = value

    return value, err
}

func (this *pebbleStorage ) GetString(key string) (string,error) {
	if val,ok := this.cache[key]; ok {
		return string(val),nil;
	}
	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	this.cache[key] = value

	return string(value),nil
}


func (this *pebbleStorage ) GetInteger(key string) (int64,error) {
	if val,ok := this.cache[key]; ok {
		return strconv.ParseInt(string(val),10,64);
	}

	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	this.cache[key] = value

	return strconv.ParseInt(string(value), 10, 64)
}

func (this *pebbleStorage ) GetBool(key string) (bool, error) {
	if val,ok := this.cache[key]; ok {
		return strconv.ParseBool(string(val));
	}
	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	this.cache[key] = value

	return strconv.ParseBool(string(value))
}

func (this *pebbleStorage ) GetFloat(key string) (float64,error) {
	if val,ok := this.cache[key]; ok {
		return strconv.ParseFloat(string(val),64);
	}

	value, _, err := this.db.Get([]byte(key))
	if err != nil {
		panic(err)
	}
	this.cache[key] = value

	return strconv.ParseFloat(string(value), 64)
}

func (this *pebbleStorage ) GetBytes(key string) ([]byte,error) {
	if val,ok := this.cache[key]; ok {
		return val,nil;
	}

	value, _, err := this.db.Get([]byte(key))
	this.cache[key] = value

	return value,err
}


func (this *pebbleStorage ) Delete(key string)  error {
  if _,ok := this.cache[key]; ok {
	  delete(this.cache,key);
  }

  return this.db.Delete([]byte(key),nil)
}

type Gauge struct {
	Value int64;
	Duration int64;
	Timestamp map[int64]int64;
}

func NewGauge(duration int64) Gauge {
	return Gauge{0,duration,make(map[int64]int64)}
}

func (this *Gauge) GetValue() int64 {
	now := time.Now().Unix();
	this.Value = 0;
	for k, v := range this.Timestamp {
		if  now - k < this.Duration {
			this.Value += v
		}
	}
	return this.Value;
}

func (this *Gauge) Add(val int64) {
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

func (this *Gauge) Inc() {
	this.Add(1);
}

func (this *Gauge) Dec() {
	this.Add(-1);
}

func (this *Gauge) Reset() {
	this.Timestamp = make(map[int64]int64);
	this.Value = 0
}

func (this *Gauge) GetDuration() int64{
	return this.Duration;
}

func GaugeAdd(ns,key,value,duration ast.Value) ( output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	lvalue, ok2 := value.(ast.Number)
	lduration, ok3 := duration.(ast.Number)
	namespace, ok4 := ns.(ast.String)

	if !ok4 {
		namespace = "gauge/default"
	} else {
		namespace = "gauge/" + namespace
	}

	if ok1 && ok2 && ok3 {
		var counter Gauge;
		lkey = namespace + "/" + lkey
		if value, err := store.GetBytes(lkey.String()) ; err == nil {
		    lduration, _ :=	lduration.Int64()
			counter = NewGauge(lduration)
			sonic.Unmarshal(value,&counter)
		} else {
			lduration, _ :=	lduration.Int64()
			counter = NewGauge(lduration)
		}
		lvalue, _ := lvalue.Int64()
		counter.Add(lvalue)
		value, _ := sonic.Marshal(counter)
		store.SetBytes(lkey.String(),value)
		output = ast.Number(fmt.Sprintf("%d", counter.GetValue()))
	} else {
		err = errors.New("Invalid input type")
		output = ast.Number("0")
	}
	return
}

func GaugeGet(ns,key ast.Value) (output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	namespace, ok2 := ns.(ast.String)
	if !ok2 {
		namespace = "gauge/default"
	} else {
		namespace = "gauge/" + namespace
	}

	if ok1 {
		lkey = namespace + "/" + lkey
		if value, err := store.GetBytes(lkey.String()) ; err == nil {
			counter := Gauge{}
			sonic.Unmarshal(value,&counter)
			output = ast.Number(fmt.Sprintf("%d", counter.GetValue()))
			return output,err
		}
	}

	return ast.Boolean(false), err
}

func GaugeDelete(ns,key ast.Value) (output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	namespace, ok2 := ns.(ast.String)
	if !ok2 {
		namespace = "gauge/default"
	} else {
		namespace = "gauge/" + namespace
	}

	if ok1 {
		lkey = namespace + "/" + lkey
		if err = store.Delete(lkey.String()); err == nil {
			return ast.Boolean(true), err
		}
	}

	return ast.Boolean(false), err
}

type Counter struct {
	Value int64;
}

func NewCounter() Counter {
	return Counter{0}
}

func (this *Counter) Add(val int64) int64 {
	this.Value += val;
	return this.Value
}

func CounterAdd(ns,key,value ast.Value) ( output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	lvalue, ok2 := value.(ast.Number)
	namespace, ok3 := ns.(ast.String)

	if !ok3 {
		namespace = "counter/default"
	} else {
		namespace = "counter/" + namespace
	}

	if ok1 && ok2 {
		var counter Counter;
		lkey = namespace + "/" + lkey
		if value, err := store.GetBytes(lkey.String()) ; err == nil {
			counter = NewCounter()
			sonic.Unmarshal(value,&counter)
		} else {
			counter = NewCounter()
		}
		lvalue, _ := lvalue.Int64()
		counter.Add(lvalue)
		value, _ := sonic.Marshal(counter)
		store.SetBytes(lkey.String(),value)
		output = ast.Number(fmt.Sprintf("%d", counter.Value))
	} else {
		err = errors.New("Invalid input type")
		output = ast.Number("0")
	}
	return
}

func CounterGet(ns,key ast.Value) (output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	namespace, ok2 := ns.(ast.String)
	if !ok2 {
		namespace = "counter/default"
	} else {
		namespace = "counter/" + namespace
	}

	if ok1 {
		lkey = namespace + "/" + lkey
		if value, err := store.GetBytes(lkey.String()) ; err == nil {
			counter := NewCounter()
			sonic.Unmarshal(value,&counter)
			output = ast.Number(fmt.Sprintf("%d", counter.Value))
			return output, err
		}
	}

	return ast.Boolean(false), err
}

func CounterDelete(ns,key ast.Value) (output ast.Value, err error){
	lkey, ok1 := key.(ast.String)
	namespace, ok2 := ns.(ast.String)
	if !ok2 {
		namespace = "counter/default"
	} else {
		namespace = "counter/" + namespace
	}

	if ok1 {
		lkey = namespace + "/" + lkey
		if err = store.Delete(lkey.String()); err == nil {
			return ast.Boolean(true), err
		}
	}

	return ast.Boolean(false), err
}

var store PersistApi;

func RegisterPebbleStore(path string){
	store = NewPebbleStorage(path,pebble.Options{})
}

func init(){
	RegisterPebbleStore("/tmp/store.db")
	RegisterFunctionalBuiltin2(ast.TimedGaugeGet.Name, GaugeGet)
	RegisterFunctionalBuiltin2(ast.TimedGaugeDelete.Name, GaugeDelete)
	RegisterFunctionalBuiltin4(ast.TimedGaugeAdd.Name, GaugeAdd)

	RegisterFunctionalBuiltin2(ast.TimedCounterGet.Name, CounterGet)
	RegisterFunctionalBuiltin2(ast.TimedCounterDelete.Name, CounterDelete)
	RegisterFunctionalBuiltin3(ast.TimedCounterAdd.Name, CounterAdd)
}