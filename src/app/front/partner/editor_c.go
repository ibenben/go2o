/**
 * Copyright 2015 @ z3q.net.
 * name : editor_c.go
 * author : jarryliu
 * date : 2015-08-18 17:09
 * description :
 * history :
 */
package partner

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jrsix/gof/web"
	"github.com/jrsix/gof/web/mvc"
	"gobx/share/variable"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

var _ sort.Interface = new(SorterFiles)

type SorterFiles struct {
	files  []os.FileInfo
	sortBy string
}

func (this *SorterFiles) Len() int {
	return len(this.files)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (this *SorterFiles) Less(i, j int) bool {
	switch this.sortBy {
	case "size":
		return this.files[i].Size() < this.files[j].Size()
	case "name":
		return this.files[i].Name() < this.files[j].Name()
	case "type":
		iN, jN := this.files[i].Name(), this.files[j].Name()
		return iN[strings.Index(iN, ".")+1:] < jN[strings.Index(jN, ".")+1:]
	}
	return true
}

// Swap swaps the elements with indexes i and j.
func (this *SorterFiles) Swap(i, j int) {
	tmp := this.files[i]
	this.files[i] = this.files[j]
	this.files[j] = tmp
}

//图片扩展名
var imgFileTypes string = "gif,jpg,jpeg,png,bmp"

// 文件管理
// @rootDir : 根目录路径，相对路径
// @rootUrl : 根目录URL，可以指定绝对路径，比如 http://www.yoursite.com/attached/
func fileManager(r *http.Request, rootDir, rootUrl string) ([]byte, error) {
	var currentPath = ""
	var currentUrl = ""
	var currentDirPath = ""
	var moveUpDirPath = ""
	var dirPath string = rootDir

	urlQuery := r.URL.Query()
	var dirName string = urlQuery.Get("dir")

	if len(dirName) != 0 {
		if dirName == "image" || dirName == "flash" ||
			dirName == "media" || dirName == "file" {
			dirPath += dirName + "/"
			rootUrl += dirName + "/"
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				os.MkdirAll(dirPath, os.ModePerm)
			}
		} else {
			return nil, errors.New("Invalid Directory name")
		}
	}

	//根据path参数，设置各路径和URL
	var path string = urlQuery.Get("path")
	if len(path) == 0 {
		currentPath = dirPath
		currentUrl = rootUrl
		currentDirPath = ""
		moveUpDirPath = ""
	} else {
		currentPath = dirPath + path
		currentUrl = rootUrl + path
		currentDirPath = path
		//reg := regexp.MustCompile("(.*?)[^\\/]+\\/$")
		moveUpDirPath = currentDirPath[:strings.LastIndex(currentDirPath, "\\")]
	}

	//不允许使用..移动到上一级目录
	if strings.Index(path, "\\.\\.") != -1 {
		return nil, errors.New("Access is not allowed.")
	}

	//最后一个字符不是/
	if path != "" && !strings.HasSuffix(path, "/") {
		return nil, errors.New("Parameter is not valid.")
	}
	//目录不存在或不是目录
	dir, err := os.Stat(currentPath)
	if os.IsNotExist(err) || !dir.IsDir() {
		return nil, errors.New("no such directory or file not directory,path:" + currentPath)
	}

	//排序形式，name or size or type
	var order string = strings.ToLower(urlQuery.Get("order"))

	//遍历目录取得文件信息

	var dirList *SorterFiles = &SorterFiles{
		files:  []os.FileInfo{},
		sortBy: order,
	}
	var fileList *SorterFiles = &SorterFiles{
		files:  []os.FileInfo{},
		sortBy: order,
	}

	files, err := ioutil.ReadDir(currentPath)
	if err != nil {
		return nil, err
	}
	for _, v := range files {
		if v.IsDir() {
			dirList.files = append(dirList.files, v)
		} else {
			fileList.files = append(fileList.files, v)
		}
	}

	var result = make(map[string]interface{})
	result["moveup_dir_path"] = moveUpDirPath
	result["current_dir_path"] = currentDirPath
	result["current_url"] = currentUrl
	result["total_count"] = dirList.Len() + fileList.Len()
	var dirFileList = []map[string]interface{}{}
	for i := 0; i < dirList.Len(); i++ {
		hash := make(map[string]interface{})
		fs, _ := ioutil.ReadDir(currentDirPath + "/" + dirList.files[i].Name())
		hash["is_dir"] = true
		hash["has_file"] = len(fs) > 0
		hash["is_photo"] = false
		hash["filetype"] = ""
		hash["filename"] = dirList.files[i].Name()
		hash["datetime"] = dirList.files[i].ModTime().Format("2006-01-02 15:04:05")
		dirFileList = append(dirFileList, hash)
	}

	var fN, ext string
	for i := 0; i < fileList.Len(); i++ {
		hash := make(map[string]interface{})
		fN = fileList.files[i].Name()
		ext = fN[strings.Index(fN, ".")+1:]
		hash["is_dir"] = false
		hash["has_file"] = false
		hash["filesize"] = fileList.files[i].Size()
		hash["is_photo"] = strings.Index(imgFileTypes, ext)
		hash["filetype"] = ext
		hash["filename"] = fN
		hash["datetime"] = fileList.files[i].ModTime().Format("2006-01-02 15:04:05")
		dirFileList = append(dirFileList, hash)
	}

	result["file_list"] = dirFileList
	return json.Marshal(result)
}

// 文件上传
func fileUpload(r *http.Request, rootDir, rootUrl string) ([]byte, error) {

	//定义允许上传的文件扩展名
	var extTable map[string]string = map[string]string{
		"image": "gif,jpg,jpeg,png,bmp",
		"flash": "swf,flv",
		"media": "swf,flv,mp3,wav,wma,wmv,mid,avi,mpg,asf,rm,rmvb",
		"file":  "doc,docx,xls,xlsx,ppt,htm,html,txt,zip,rar,gz,bz2,7z,pdf",
	}

	//最大文件大小
	const maxSize int = 1000000

	// 取得上传文件
	r.ParseMultipartForm(maxSize)
	f, header, err := r.FormFile("imgFile")
	if f == nil {
		return nil, errors.New("no such upload file")
	}
	if err != nil {
		return nil, err
	}

	var fileName string = header.Filename
	var fileExt string = strings.ToLower(fileName[strings.Index(fileName, ".")+1:])

	// 检查上传目录
	var dirPath string = rootDir
	var dirName string = r.URL.Query().Get("dir")
	if len(dirName) == 0 {
		dirName = "image"
	}
	if _, ok := extTable[dirName]; !ok {
		return nil, errors.New("incorrent file type")
	}

	// 检查扩展名
	if strings.Index(extTable[dirName], fileExt) == -1 &&
		!strings.HasSuffix(extTable[dirName], fileExt) {
		return nil, errors.New("上传文件扩展名是不允许的扩展名。\n只允许" + extTable[dirName] + "格式。")
	}

	// 检查上传超出文件大小
	if i, _ := strconv.Atoi(header.Header.Get("Content-Length")); i > maxSize {
		return nil, errors.New("上传文件大小超过限制。")
	}

	/*
	   //创建文件夹
	   dirPath += dirName + "/";
	   saveUrl += dirName + "/";
	   if (!Directory.Exists(dirPath))
	   {
	       Directory.CreateDirectory(dirPath).Create();
	   }
	   String ymd = DateTime.Now.ToString("yyyyMM", DateTimeFormatInfo.InvariantInfo);
	   dirPath += ymd + "/";
	   saveUrl += ymd + "/";
	   if (!Directory.Exists(dirPath))
	   {
	       Directory.CreateDirectory(dirPath);
	   }

	   String newFileName = DateTime.Now.ToString("yyyyMMddHHmmss_ffff", DateTimeFormatInfo.InvariantInfo) +
	                        fileExt;
	   String filePath = dirPath + newFileName;

	   imgFile.SaveAs(filePath);

	   String fileUrl = saveUrl + newFileName;

	   Hashtable hash = new Hashtable();
	   hash["error"] = 0;
	   hash["url"] = fileUrl;
	   context.Response.AddHeader("Content-Type", "text/html; charset=UTF-8");


	   context.Response.Write(JsonAnalyzer.ToJson(hash));
	   context.Response.End();
	*/
}

var _ mvc.Filter = new(editorC)

type editorC struct {
	*baseC
}

func (this *editorC) File_manager(ctx *web.Context) {
	partnerId := this.GetPartnerId(ctx)
	d, err := fileManager(ctx.Request,
		fmt.Sprintf("./static/uploads/%d/upload/", partnerId),
		fmt.Sprintf("%s/%d/upload/", ctx.App.Config().GetString(variable.StaticServer), partnerId),
	)
	ctx.Response.Header().Add("Content-Type", "application/json")
	if err != nil {
		ctx.Response.Write([]byte("{error:'" + strings.Replace(err.Error(), "'", "\\'", -1) + "'}"))
	} else {
		ctx.Response.Write(d)
	}
}

func (this *editorC) File_upload(ctx *web.Context) {
	partnerId := this.GetPartnerId(ctx)
	d, err := fileUpload(ctx.Request,
		fmt.Sprintf("./static/uploads/%d/upload/", partnerId),
		fmt.Sprintf("%s/%d/upload/", ctx.App.Config().GetString(variable.StaticServer), partnerId),
	)
	ctx.Response.Header().Add("Content-Type", "application/json")
	if err != nil {
		ctx.Response.Write([]byte("{error:'" + strings.Replace(err.Error(), "'", "\\'", -1) + "'}"))
	} else {
		ctx.Response.Write(d)
	}
}
