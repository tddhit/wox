package wox

import (
	"bytes"
	"sync"
)

type stats struct {
	sync.Mutex
	Master *processStats         `json:"master"`
	Worker map[int]*processStats `json:"worker"`
	html   string
}

type processStats struct {
	Id     int            `json:"id"`
	QPS    int            `json:"qps"`
	Method map[string]int `json:"method"`
}

func newStats() *stats {
	s := &stats{
		Master: &processStats{
			Method: make(map[string]int),
		},
		Worker: make(map[int]*processStats),
	}
	var buf bytes.Buffer
	buf.WriteString(statsBeginHTML)
	buf.WriteString(statsJS)
	buf.WriteString(statsEndHTML)
	s.html = buf.String()
	return s
}

func (s *stats) resetMaster() {
	s.Master.Id = 0
	s.Master.QPS = 0
	for name, _ := range s.Master.Method {
		s.Master.Method[name] = 0
	}
}

func (s *stats) resetWorker(pid int) {
	s.Worker[pid].Id = 0
	s.Worker[pid].QPS = 0
	for name, _ := range s.Worker[pid].Method {
		s.Worker[pid].Method[name] = 0
	}
}

const statsJS = `
var first=true;var charts=new Array;var options=new Array;for(var i=0;i<3;i++){var chart=echarts.init(document.getElementById("chart-"+i));charts.push(chart)}var Option=function(){return{title:{text:""},tooltip:{trigger:"axis",formatter:function(params){params=params[0];var date=new Date(params.name);return date.getDate()+"/"+(date.getMonth()+1)+"/"+date.getFullYear()+" : "+params.value[1]},axisPointer:{animation:false}},legend:{data:[]},xAxis:{type:"time",splitLine:{show:false}},yAxis:{type:"value",axisLabel:{formatter:"{value} qps"},boundaryGap:[0,"100%"],splitLine:{show:false}},series:[]}};var SeriesItem=function(){return{name:"",type:"line",showSymbol:false,hoverAnimation:false,smooth:true,data:[],markPoint:{data:[{type:"max",name:"最大值"},{type:"min",name:"最小值"}]},markLine:{data:[{type:"average",name:"平均值"}]}}};var ProcessStats=function(){return{id:0,methods:[]}};var MethodStats=function(){return{name:"",qps:-1}};function Convert(data){var processes=new Array;var process=new ProcessStats;process.id=data.master.id;var method=new MethodStats;method.name="Total";method.qps=data.master.qps;process.methods.push(method);for(var name in data.master.method){var method=new MethodStats;method.name=name;method.qps=data.master.method[name];process.methods.push(method)}processes.push(process);for(var pid in data.worker){var worker=data.worker[pid];var process=new ProcessStats;process.id=pid;var method=new MethodStats;method.name="Total";method.qps=worker.qps;process.methods.push(method);for(var name in worker.method){var method=new MethodStats;method.name=name;method.qps=worker.method[name];process.methods.push(method)}processes.push(process)}return processes}function GetStats(){jQuery.ajax({url:"http://##ListenAddr##/stats",type:"get",async:"true",dataType:"json",success:function(data){var newData=Convert(data);if(!first){if(newData[0].methods.length!=options[0].series.length){first=true}else if("Master-"+newData[0].id!=options[0].title.text){first=true}for(var i=1;i<newData.length;i++){if(newData[i].methods.length!=options[i].series.length){first=true}else if("Worker-"+newData[i].id!=options[i].title.text){first=true}}}if(first){first=false;options.splice(0,options.length);for(var i in newData){var option=new Option;var process=newData[i];if(i==0){title="Master-"+process.id}else{title="Worker-"+process.id}option.title.text=title;for(var j in process.methods){var method=process.methods[j];option.legend.data.push(method.name);var seriesItem=new SeriesItem;seriesItem.name=method.name;var now=new Date;value={name:now.toString(),value:[now,method.qps]};seriesItem.data.push(value);option.series.push(seriesItem)}options.push(option);charts[i].setOption(option,true)}}else{for(var i in newData){var option=options[i];var process=newData[i];for(var j in process.methods){var method=process.methods[j];var now=new Date;value={name:now.toString(),value:[now,method.qps]};option.series[j].data.push(value)}charts[i].setOption(option,true);for(var j in process.methods){if(option.series[j].data.length==20){option.series[j].data.shift()}}}}}})}setInterval(GetStats,1e3);
`

const statsBeginHTML = `
<!DOCTYPE html><html style="height: 100%"><head><meta charset="utf-8"><title>Wox性能监控</title></head><body style="height: 100%; margin: 0"><div id="chart-0"style="height:400px"></div><div id="chart-1"style="height:400px"></div><div id="chart-2"style="height:400px"></div><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts/echarts.min.js"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts-gl/echarts-gl.min.js"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts-stat/ecStat.min.js"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts/extension/dataTool.min.js"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts/map/js/china.js"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts/map/js/world.js"></script><script type="text/javascript"src="http://api.map.baidu.com/api?v=2.0&ak=ZUONbpqGBsYGXNIYHicvbAbM"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/echarts/extension/bmap.min.js"></script><script type="text/javascript"src="http://echarts.baidu.com/gallery/vendors/simplex.js"></script><script type="text/javascript"src="http://code.jquery.com/jquery-latest.js"></script><script type="text/javascript">
`

const statsEndHTML = `
</script></body></html>
`
