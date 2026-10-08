### 结构

```json
{
  "type": "urltest",
  "tag": "auto",
  
  "outbounds": [
    "proxy-a",
    "proxy-b",
    "proxy-c"
  ],
  "url": "",
  "interval": "",
  "tolerance": 50,
  "idle_timeout": "",
  "interrupt_exist_connections": false,
  "lazy_start": false
}
```

### 字段

#### outbounds

==必填==

用于测试的出站标签列表。

#### url

用于测试的链接。默认使用 `https://www.gstatic.com/generate_204`。

#### interval

测试间隔。 默认使用 `3m`。

#### tolerance

以毫秒为单位的测试容差。 默认使用 `50`。

#### idle_timeout

空闲超时。默认使用 `30m`。

#### interrupt_exist_connections

当选定的出站发生更改时，中断现有连接。

仅入站连接受此设置影响，内部连接将始终被中断。

#### lazy_start

`sing-box-lx` 扩展，默认 `false`。

开启后，尚无流量经过的组不执行自身的启动和网络变化测速。首次连接或
连接附加会立即安排一次测试，流量先走原有的冷启动节点／候选池。
网络变化只重测仍有活动计时器的组；空闲超时后，下次流量会重新立即测试。
读取状态不会启动测试。自动测试遵守设备与网络暂停；醒来后的首次有效流量
会重试尚未执行的初测。

手动测速仍然强制测试，也不会开启后台计时器。活动的父 URLtest 组为了选择
节点，仍可能测试尚未使用的子组。此选项减少后台探测，实际节点延迟与电量
收益仍需分别测量。
