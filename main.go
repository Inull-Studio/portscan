package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

var (
	Color = struct {
		N, RED, GREEN string
	}{
		RED:   "\033[0;31m",
		GREEN: "\033[0;32m",
		N:     "\033[0m"}
	startTime = time.Now()
	ip        = flag.String("ip", "", "ip 例如:-ip 192.168.1.123,10.10.0.3 或 -ip 192.168.1.1-123,10.10.10.3-254")
	port      = flag.String("p", "22-1000", "端口号范围 例如:-p 80,81,88-1000")
	timeout   = flag.Int("t", 200, "超时时长(毫秒) 例如:-t 200")
	h         = flag.Bool("h", false, "帮助信息")
	maxConns  = 4000
	slowMode  = flag.Bool("slow", false, "慢速模式，防止连接数超过系统限制")
)

type ScanIp struct {
	debug       bool
	timeout     int
	connLimiter chan struct{}
}

func (s *ScanIp) initConnLimiter(maxConns int) {
	s.connLimiter = make(chan struct{}, maxConns)
}
func (s *ScanIp) filterPort(str string) (int, error) {
	port, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	if port < 1 || port > 65535 {
		return 0, errors.New("端口号超出范围 1-65535")
	}
	return port, nil
}
func (s *ScanIp) getAllPort(port string) ([]int, error) {
	var ports []int
	//处理 ","号 如 80,81,88 或 80,88-100
	portArr := strings.Split(strings.Trim(port, ","), ",")
	for _, v := range portArr {
		portArr2 := strings.Split(strings.Trim(v, "-"), "-")
		startPort, err := s.filterPort(portArr2[0])
		if err != nil {
			fmt.Println(err)
			continue
		}
		//第一个端口先添加
		ports = append(ports, startPort)
		if len(portArr2) > 1 {
			//添加第一个后面的所有端口
			endPort, _ := s.filterPort(portArr2[1])
			endPort++
			if endPort > startPort {
				for i := 1; i < endPort-startPort; i++ {
					ports = append(ports, startPort+i)
				}
			}
		}
	}
	//去重复
	ports = s.arrayUnique(ports)
	return ports, nil
}
func (s *ScanIp) ipUp(ip string) bool {
	con, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return false
	}
	defer con.Close()
	m := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{ID: 1, Seq: 1, Data: []byte{}},
	}
	b, err := m.Marshal(nil)
	if err != nil {
		return false
	}
	_, err = con.WriteTo(b, &net.IPAddr{IP: net.ParseIP(ip)})
	if err != nil {
		return false
	}
	rb := make([]byte, 1500)
	err = con.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(*timeout)))
	if err != nil {
		return false
	}
	n, _, err := con.ReadFrom(rb)
	if err != nil {
		return false
	} else {
		rm, err := icmp.ParseMessage(1, rb[:n])
		if err != nil {
			return false
		}
		switch rm.Type {
		case ipv4.ICMPTypeEchoReply:
			con.Close()
			return true
		default:
			con.Close()
			return false
		}
	}
}

func (s *ScanIp) arrayUnique(arr []int) []int {
	var newArr []int
	for i := 0; i < len(arr); i++ {
		repeat := false
		for j := i + 1; j < len(arr); j++ {
			if arr[i] == arr[j] {
				repeat = true
				break
			}
		}
		if !repeat {
			newArr = append(newArr, arr[i])
		}
	}
	return newArr
}

func (s *ScanIp) isOpen(ip string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), time.Duration(s.timeout*int(time.Millisecond)))
	if err != nil {
		if strings.Contains(err.Error(), "too many open files") {
			fmt.Println("连接数超出系统限制!")
			os.Exit(1)
		}
		return false
	}
	defer conn.Close()
	return true
}
func (s *ScanIp) GetIpOpenPort(ip string, port string) []int {
	var (
		openPorts []int
		mutex     sync.Mutex
	)
	ports, _ := s.getAllPort(port)
	if !s.ipUp(ip) {
		return []int{}
	}
	wg := sync.WaitGroup{}
	for _, p := range ports {
		wg.Add(1)
		go func(port int) {
			s.connLimiter <- struct{}{}
			defer func() {
				<-s.connLimiter // 释放连接槽位
			}()
			opened := s.isOpen(ip, port)
			mutex.Lock()
			if opened {
				openPorts = append(openPorts, port)
				fmt.Printf(Color.GREEN+"open"+Color.N+"\t%v:%v\r\n", ip, port)
			}
			wg.Done()
			mutex.Unlock()
		}(p)
	}
	wg.Wait()
	return openPorts
}
func (s *ScanIp) GetAllIp(ip string) ([]string, error) {
	var ips []string
	tmpip := strings.Split(ip, ",")
	for _, ip := range tmpip {
		ipTmp := strings.Split(ip, "-")
		firstIp, err := net.ResolveIPAddr("ip", ipTmp[0])
		if err != nil {
			return ips, errors.New(ipTmp[0] + "域名解析失败" + err.Error())
		}
		if net.ParseIP(firstIp.String()) == nil {
			return ips, errors.New(ipTmp[0] + " ip地址有误")
		}
		//域名转化成ip再塞回去
		ipTmp[0] = firstIp.String()
		ips = append(ips, ipTmp[0]) //最少有一个ip地址

		if len(ipTmp) == 2 {
			//以切割第一段ip取到最后一位
			ipTmp2 := strings.Split(ipTmp[0], ".")
			startIp, _ := strconv.Atoi(ipTmp2[3])
			endIp, err := strconv.Atoi(ipTmp[1])
			if err != nil || endIp < startIp {
				endIp = startIp
			}
			if endIp > 255 {
				endIp = 255
			}
			totalIp := endIp - startIp + 1
			for i := 1; i < totalIp; i++ {
				ips = append(ips, fmt.Sprintf("%s.%s.%s.%d", ipTmp2[0], ipTmp2[1], ipTmp2[2], startIp+i))
			}
		}
	}
	return ips, nil
}

func main() {
	fmt.Printf("Start %v \n", time.Now().Format(time.UnixDate))
	runtime.GOMAXPROCS(4)
	flag.Parse()
	scanIP := ScanIp{
		debug:   true,
		timeout: *timeout,
	}
	scanIP.initConnLimiter(maxConns)
	//帮助信息
	if *h {
		flag.Usage()
		return
	}
	if *slowMode {
		scanIP.initConnLimiter(1000)
	}
	if strings.EqualFold(strings.Trim(*ip, " "), "") {
		flag.Usage()
	}
	ips, _ := scanIP.GetAllIp(*ip)
	for _, ip := range ips {
		openports := scanIP.GetIpOpenPort(ip, *port)
		fmt.Printf("%s 开启端口数: %d\n", ip, len(openports))
	}
	//初始化
	fmt.Printf("End %v 执行时长 : %.2fs \n", time.Now().Format(time.UnixDate), time.Since(startTime).Seconds())
}
