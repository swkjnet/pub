package pub

import (
	"errors"
	"fmt"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	ERR_DBCONNECT = "connect db error"
	ERR_HASINDEX  = "index has exist"
)

const (
	dblog        = "DBlog"
	dblogMonitor = "dblogmonitor"
)

func InitDatabase(url string) (*DataBase, error) {
	db := new(DataBase)
	if !db.InitPool(url) {
		return nil, errors.New("db initpool error")
	}
	return db, nil
}

type DBSession struct {
	*mgo.Session
	Active bool
}
type DataBase struct {
	pool    chan *DBSession
	url     string //url
	curNum  int    //当前会话数
	maxNum  int    //最大会话数
	reqNum  int    //请求数
	cost    int    //总耗时
	maxCost int    //最大耗时
	lastT   int64  //lastt
}

type PageData struct {
	Total int      `json:"total"`
	Rows  []bson.M `json:"rows"`
	Page  int      `json:"page"`
}

func GetUID() string { ///分配一个唯一ID
	return bson.NewObjectId().Hex()
}

//初始化链接池
func (this *DataBase) InitPool(url string) bool {
	this.url = url
	this.lastT = time.Now().Unix() + 3600
	this.pool = make(chan *DBSession, 100)
	sess := this.getSession()
	if sess == nil {
		return false
	}
	this.freeSession(sess)
	return true
}

//运行明细
func (this *DataBase) Monitor() {
	if time.Now().Unix() < this.lastT {
		return
	}
	this.lastT = time.Now().Unix() + 3600
	avgcost := 0
	if this.reqNum > 0 {
		avgcost = this.cost / this.reqNum
	}
	PrintFileLog(dblogMonitor, fmt.Sprintf("cur_session_num:%d,max_session_num:%d,request_num:%d,request_maxcost:%d,request_avg_cost:%d,url:%s",
		this.curNum, this.maxNum, this.reqNum, this.maxCost, avgcost, this.url))
}

//警告
func (this *DataBase) Warning(func_name string, dbname string, colname string, desc interface{}, start time.Time) {
	cost := int(time.Since(start) / time.Millisecond)
	this.reqNum++
	this.cost += cost
	if cost > this.maxCost {
		this.maxCost = cost
	}
	if this.cost > 1000 {
		PrintFileLog(dblogMonitor, "overtime ", func_name, ",dbname:", dbname, ",colname:", colname, ",desc:", desc, ",cost:", this.cost)
	}
}

//获取会话
func (this *DataBase) getSession() *DBSession {
	this.curNum++
	if this.curNum > this.maxNum {
		this.maxNum = this.curNum
	}
	select {
	case v := <-this.pool:
		return v
	default:
		sess, err := mgo.Dial(this.url)
		if err != nil {
			PrintFileLog(dblog, fmt.Sprintf("connect db error:%s", err.Error()))
			this.curNum--
			return nil
		}
		sess.SetCursorTimeout(0)
		sess.SetSocketTimeout(0)
		sess.SetSyncTimeout(0)
		return &DBSession{Session: sess, Active: true}
	}
}

//释放会话
func (this *DataBase) freeSession(sess *DBSession) {
	this.curNum--
	this.Monitor()
	if sess == nil {
		return
	}
	if !sess.Active {
		sess.Close()
		PrintFileLog(dblog, "Close the exception dbconn url:", this.url)
		return
	}
	select {
	case this.pool <- sess:
	default:
		sess.Close()
		PrintFileLog(dblog, "dbpool is full url:", this.url)
	}
}

//创建索引
func (this *DataBase) IndexTable(dbname, colname string, indexname string, key []string, unique bool, dropDups bool) error {
	sess := this.getSession()
	if sess == nil {
		return errors.New(ERR_DBCONNECT)
	}
	defer this.freeSession(sess)
	db := sess.DB(dbname)
	col := db.C(colname)
	ins, _ := col.Indexes()
	for _, v := range ins {
		if v.Name == indexname {
			return errors.New(ERR_HASINDEX)
		}
	}
	index := mgo.Index{
		Key:        key,
		Unique:     unique,
		DropDups:   dropDups,
		Background: true, // See notes.
		Sparse:     false,
		Name:       indexname,
	}
	err := col.EnsureIndex(index)
	if err != nil {
		PrintFileLog(dblog, fmt.Sprintf("creat index error:%s", err.Error()))
		sess.Active = false
		return err
	}
	return nil
}

//按条件查询记录数
func (this *DataBase) FindCount(dbname, colname string, find interface{}) int {
	sess := this.getSession()
	if sess == nil {
		return 0
	}
	defer this.freeSession(sess)
	defer this.Warning("findcount", dbname, colname, find, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	count, err := col.Find(find).Count()
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("FindCount error:%s", err.Error()))
		sess.Active = false
		return 0
	}
	return count
}

//按条件按列键查询
func (this *DataBase) FindAllSelector(dbname, colname string, find interface{}, selector interface{}, result interface{}) error {
	sess := this.getSession()
	if sess == nil {
		return errors.New(ERR_DBCONNECT)
	}
	defer this.freeSession(sess)
	defer this.Warning("FindAllSelector", dbname, colname, find, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.Find(find).Select(selector).All(result)
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("FindAllSelector error:%s", err.Error()))
		sess.Active = false
		return err
	}
	return nil
}

//查询所有表
func (this *DataBase) GetCollectionNames(dbname string) ([]string, error) {
	sess := this.getSession()
	if sess == nil {
		return nil, errors.New(ERR_DBCONNECT)
	}
	defer this.freeSession(sess)
	db := sess.DB(dbname)
	names, err := db.CollectionNames()
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("GetCollectionNames error:%s", err.Error()))
		sess.Active = false
		return nil, err
	}
	return names, nil
}

//按条件查询
func (this *DataBase) FindAll(dbname, colname string, find interface{}, result interface{}) error {
	sess := this.getSession()
	if sess == nil {
		return errors.New(ERR_DBCONNECT)
	}
	defer this.freeSession(sess)
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.Find(find).All(result)
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("FindAll error:%s", err.Error()))
		sess.Active = false
		return err
	}
	return nil
}

//根据条件获取一条记录
func (this *DataBase) FindOne(dbname, colname string, find interface{}, result interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("FindOne", dbname, colname, find, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.Find(find).One(result)
	if err != nil {
		if err.Error() != "not found" {
			PrintFileLog(dblog, fmt.Sprintf("FindOne error:%s", err.Error()))
			sess.Active = false
		}
		return false
	}
	return true
}

//根据id查找
func (this *DataBase) FindId(dbname, colname string, id interface{}, result interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("FindId", dbname, colname, id, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.FindId(id).One(result)
	if err != nil {
		if err.Error() != "not found" {
			PrintFileLog(dblog, fmt.Sprintf("FindId error:%s", err.Error()))
			sess.Active = false
		}
		return false
	}
	return true
}

//游标遍历处理
func (this *DataBase) FindIter(dbname, colname string, find interface{}, result interface{}, iterFunc func(int) bool, sorfields ...string) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	col := sess.DB(dbname).C(colname)
	iter := col.Find(find).Sort(sorfields...).Iter()
	i := 0
	for iter.Next(result) {
		if !iterFunc(i) {
			break
		}
		i++
	}
	return true
}

//分页查询
func (this *DataBase) FindBySkipLimit(dbname, colname string, find interface{}, result interface{}, skip int, limit int, sortFields ...string) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("FindBySkipLimit", dbname, colname, find, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	var err error
	if len(sortFields) == 0 {
		err = col.Find(find).Skip(skip).Limit(limit).All(result)
	} else {
		err = col.Find(find).Skip(skip).Sort(sortFields...).Limit(limit).All(result)
	}
	if err != nil {
		if err.Error() != "not found" {
			PrintFileLog(dblog, fmt.Sprintf("FindBySkipLimit error:%s", err.Error()))
			sess.Active = false
		}
		return false
	}
	return true
}

//更新
func (this *DataBase) Update(dbname, colname string, selector interface{}, update interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("Update", dbname, colname, selector, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.Update(selector, bson.M{"$set": update})
	if err != nil {
		PrintFileLog(dblog, fmt.Sprintf("Update error:%s", err.Error(), " selector:", selector, " update:", update))
		sess.Active = false
		PrintFileLog(dblog, "session error:", sess.Active)
		return false
	}
	return true
}

//按条件更新，若没有则插入
func (this *DataBase) UpdateNoInsert(dbname, colname string, selector interface{}, update interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("UpdateNoInsert", dbname, colname, selector, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.Update(selector, bson.M{"$set": update})
	if err != nil {
		if err.Error() == "not found" {
			col.Insert(update)
		} else {
			PrintFileLog(dblog, fmt.Sprintf("UpdateNoInsert error:%s", err.Error()))
			sess.Active = false
			return false
		}
	}
	return true
}

//按唯一索引更新，没有则插入
func (this *DataBase) Upsert(dbname, colname string, selector interface{}, update interface{}) (*mgo.ChangeInfo, bool) {
	sess := this.getSession()
	if sess == nil {
		return nil, false
	}
	defer this.freeSession(sess)
	defer this.Warning("Upsert", dbname, colname, selector, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	info, err := col.Upsert(selector, update)
	if err != nil {
		PrintFileLog(dblog, fmt.Sprintf("Upsert error:%s", err.Error()))
		sess.Active = false
		return nil, false
	}
	return info, true
}

//更新所有
func (this *DataBase) UpdateAll(dbname, colname string, selector interface{}, update interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("UpdateAll", dbname, colname, selector, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	_, err := col.UpdateAll(selector, bson.M{"$set": update})
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("UpdateAll error:%s", err.Error()))
		sess.Active = false
		return false
	}
	return true
}

//根据ID更新
func (this *DataBase) Updatebyid(dbname, colname string, id interface{}, update interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("Updatebyid", dbname, colname, id, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.UpdateId(id, bson.M{"$set": update})
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("UpdateByid error:%s", err.Error()))
		sess.Active = false
		return false
	}
	return true
}

//删除
func (this *DataBase) Delete(dbname, colname string, id interface{}) error {
	sess := this.getSession()
	if sess == nil {
		return errors.New(ERR_DBCONNECT)
	}
	defer this.freeSession(sess)
	defer this.Warning("Delete", dbname, colname, id, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.RemoveId(id)
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("Delete error:%s", err.Error()))
		sess.Active = false
		return err
	}
	return nil
}

//删除表
func (this *DataBase) DropCol(dbname, colname string) error {
	sess := this.getSession()
	if sess == nil {
		return errors.New(ERR_DBCONNECT)
	}
	defer this.freeSession(sess)
	defer this.Warning("DropCol", dbname, colname, "", time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.DropCollection()
	if err != nil && err.Error() != "not found" {
		PrintFileLog(dblog, fmt.Sprintf("drop error:%s", err.Error()))
		sess.Active = false
		return err
	}
	return nil
}

//插入记录
func (this *DataBase) Insert(dbname, colname string, data interface{}) bool {
	sess := this.getSession()
	if sess == nil {
		return false
	}
	defer this.freeSession(sess)
	defer this.Warning("Insert", dbname, colname, data, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	err := col.Insert(data)
	if err != nil {
		PrintFileLog(dblog, fmt.Sprintf("Insert error:%s", err.Error()))
		sess.Active = false
		return false
	}
	return true
}

//分页处理
func (this *DataBase) GetPage(dbname, colname string, find interface{}, fields []string, sort []string, page int, rows int) *PageData {
	data := "find PageData"
	sess := this.getSession()
	if sess == nil {
		return nil
	}
	defer this.freeSession(sess)
	defer this.Warning("Find", dbname, colname, data, time.Now())
	db := sess.DB(dbname)
	col := db.C(colname)
	if page == 0 {
		page = 1
	}
	if rows == 0 {
		rows = 20
	}
	res := new(PageData)
	res.Page = page
	res.Rows = make([]bson.M, 0)
	q := col.Find(find)
	var err error
	res.Total, err = q.Count()
	if err != nil {
		return nil
	}
	if fields != nil {
		var Mfields bson.M = make(bson.M)
		for _, v := range fields {
			Mfields[v] = 1
		}
		q = q.Select(Mfields)
	}
	if sort != nil {
		q.Sort(sort...)
	}
	err = q.Skip((page - 1) * rows).Limit(rows).All(&res.Rows)
	if err != nil {
		return nil
	}
	return res
}
