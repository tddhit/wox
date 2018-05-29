var first = true;
var charts = new Array();
var options = new Array();
for (var i = 0; i < 3; i++) {
    var chart = echarts.init(document.getElementById('chart-' + i));
    charts.push(chart);
}
var Option = function() {
    return {
        title: {
            text: ''
        },
        tooltip: {
            trigger: 'axis',
            formatter: function(params) {
                params = params[0];
                var date = new Date(params.name);
                return date.getDate() + '/' + (date.getMonth() + 1) + '/' + date.getFullYear() + ' : ' + params.value[1];
            },
            axisPointer: {
                animation: false
            }
        },
        legend: {
            data: []
        },
        xAxis: {
            type: 'time',
            splitLine: {
                show: false
            }
        },
        yAxis: {
            type: 'value',
            axisLabel: {
                formatter: '{value} qps'
            },
            boundaryGap: [0, '100%'],
            splitLine: {
                show: false
            }
        },
        series: []
    }
}
var SeriesItem = function() {
    return {
        name: '',
        type: 'line',
        showSymbol: false,
        hoverAnimation: false,
        smooth: true,
        data: [],
        markPoint: {
            data: [{
                type: 'max',
                name: '最大值'
            },
            {
                type: 'min',
                name: '最小值'
            }]
        },
        markLine: {
            data: [{
                type: 'average',
                name: '平均值'
            }]
        },
    }
}
var ProcessStats = function() {
    return {
        id: 0,
        methods: [],
    }
}
var MethodStats = function() {
    return {
        name: '',
        qps: -1,
    }
}
function Convert(data) {
    var processes = new Array();
    var process = new ProcessStats();
    process.id = data.master.id;
    var method = new MethodStats();
    method.name = 'Total';
    method.qps = data.master.qps;
    process.methods.push(method);
    for (var name in data.master.method) {
        var method = new MethodStats();
        method.name = name;
        method.qps = data.master.method[name];
        process.methods.push(method);
    }
    processes.push(process);
    for (var pid in data.worker) {
        var worker = data.worker[pid];
        var process = new ProcessStats();
        process.id = pid;
        var method = new MethodStats();
        method.name = 'Total';
        method.qps = worker.qps;
        process.methods.push(method);
        for (var name in worker.method) {
            var method = new MethodStats();
            method.name = name;
            method.qps = worker.method[name];
            process.methods.push(method);
        }
        processes.push(process);
    }
    return processes
}
function GetStats() {
    jQuery.ajax({
        url: "http://172.17.202.212:18861/stats",
        type: 'get',
        async: 'true',
        dataType: 'json',
        success: function(data) {
            var newData = Convert(data);
            if (!first) {
                if (newData[0].methods.length != options[0].series.length) {
                    first = true
                } else if (newData[1].methods.length != options[1].series.length) {
                    first = true
                } else if (newData[2].methods.length != options[2].series.length) {
                    first = true
                } else if (('Master-'+newData[0].id) != options[0].title.text) {
                    first = true
                } else if (('Worker-'+newData[1].id) != options[1].title.text) {
                    first = true
                } else if (('Worker-'+newData[2].id) != options[2].title.text) {
                    first = true
                }
            }
            if (first) {
                first = false;
                options.splice(0, options.length);
                for (var i in newData) {
                    if (i > 3) {
                        continue
                    }
                    var option = new Option();
                    var process = newData[i];
                    if (i == 0) {
                        title = 'Master-' + process.id;
                    } else {
                        title = 'Worker-' + process.id;
                    }
                    option.title.text = title;
                    for (var j in process.methods) {
                        var method = process.methods[j];
                        option.legend.data.push(method.name);
                        var seriesItem = new SeriesItem();
                        seriesItem.name = method.name;
                        var now = new Date() 
                        value = {
                            name: now.toString(),
                            value: [now, method.qps]
                        }
                        seriesItem.data.push(value);
                        option.series.push(seriesItem);
                    }
                    options.push(option);
                    charts[i].setOption(option, true);
                }
            } else {
                for (var i in newData) {
                    if (i > 3) {
                        continue
                    }
                    var option = options[i];
                    var process = newData[i];
                    for (var j in process.methods) {
                        var method = process.methods[j];
                        var now = new Date();
                        value = {
                            name: now.toString(),
                            value: [now, method.qps]
                        }
                        option.series[j].data.push(value);
                    }
                    charts[i].setOption(option, true);
                    for (var j in process.methods) {
                        if (option.series[j].data.length == 20) {
                            option.series[j].data.shift()
                        }
                    }
                }
            }
        }
    })
}
setInterval(GetStats, 1000)
