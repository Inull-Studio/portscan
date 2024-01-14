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
	maxConns = 10000
	ip       = flag.String("ip", "", "ip 例如:-ip 192.168.1.123,10.10.0.3 或 -ip 192.168.1.1-123,10.10.10.3-254")
	port     = flag.String("p", "22-1000", "端口号范围 例如:-p 80,81,88-1000")
	timeout  = flag.Int("t", 200, "超时时长(毫秒) 例如:-t 200")
	h        = flag.Bool("h", false, "帮助信息")
	slowMode = flag.Bool("slow", false, "慢速模式，防止连接数超过系统限制")
	noping   = flag.Bool("np", false, "不使用主机发现")
	verbose  = flag.Bool("v", false, "详细信息")
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
			os.Exit(1)
		}
		//第一个端口先添加
		ports = append(ports, startPort)
		if len(portArr2) > 1 {
			//添加第一个后面的所有端口
			endPort, err := s.filterPort(portArr2[1])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
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

	addr, _ := net.ResolveIPAddr("ip", ip)
	_, err = con.WriteTo(b, addr)
	if err != nil {
		return false
	}
	rb := make([]byte, 1500)
	err = con.SetReadDeadline(time.Now().Add(time.Duration(s.timeout * 2)))
	if err != nil {
		return false
	}
	n, _, err := con.ReadFrom(rb)
	if err != nil {
		return false
	}
	rm, err := icmp.ParseMessage(1, rb[:n])
	if err != nil {
		return false
	}
	switch rm.Type {
	default:
		if *verbose {
			fmt.Printf("%s ICMP存活okok\n", ip)
		}
		return true
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
			fmt.Println("连接数超出系统限制! 尝试-slow慢速模式")
			os.Exit(1)
		} else if strings.Contains(err.Error(), "lacked sufficient buffer space") {
			fmt.Println("网络缓冲区已满! 尝试-slow慢速模式")
			os.Exit(1)
		}
		return false
	}
	defer conn.Close()
	return true
}
func (s *ScanIp) GetIpOpenPort(ip string, port string) ([]int, error) {
	var (
		openPorts []int
		mutex     sync.Mutex
	)
	ports, _ := s.getAllPort(port)
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
	return openPorts, nil
}
func (s *ScanIp) GetAllIp(ip string) ([]string, error) {
	var (
		mutex sync.Mutex
		ips   []string
	)
	if !strings.Contains(ip, ".") {
		return ips, errors.New("未知IP")
	}
	tmpip := strings.Split(ip, ",")
	wg := sync.WaitGroup{}
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
		if len(ipTmp) == 1 {
			ipTmp = append(ipTmp, firstIp.String())
		}

		//以切割第一段ip取到最后一位
		ipTmp2 := strings.Split(ipTmp[0], ".")
		startIp, _ := strconv.Atoi(ipTmp2[3])
		endIp, err := strconv.Atoi(ipTmp[1])
		if err != nil || endIp < startIp {
			endIp = startIp
		}
		if endIp > 255 {
			return ips, errors.New("IP地址范围 1-255")
		}
		totalIp := endIp - startIp + 1
		//ICMP存活检测
		for i := 0; i < totalIp; i++ {
			wg.Add(1)
			go func(i int) {
				ip := fmt.Sprintf("%s.%s.%s.%d", ipTmp2[0], ipTmp2[1], ipTmp2[2], startIp+i)
				mutex.Lock()
				if !*noping {
					if s.ipUp(ip) {
						ips = append(ips, ip)
					}
				} else {
					ips = append(ips, ip)
				}
				mutex.Unlock()
				wg.Done()
			}(i)
		}
	}
	wg.Wait()
	return ips, nil
}

func main() {
	startTime := time.Now()
	fmt.Printf("Start %s \n", startTime.Format("2006-01-02 15:04:05"))
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
	if *noping {
		fmt.Println(Color.RED + "开启noping选项，扫描时间或许会大幅增加" + Color.N)
	}
	if *slowMode {
		scanIP.initConnLimiter(1000)
	}
	if strings.EqualFold(strings.Trim(*ip, " "), "") {
		flag.Usage()
	}
	ips, _ := scanIP.GetAllIp(*ip)
	for _, ip := range ips {
		openports, err := scanIP.GetIpOpenPort(ip, *port)
		if err != nil {
			continue
		}
		if len(openports) != 0 {
			fmt.Printf("%s 开启端口数: %d\n", ip, len(openports))
		}
		runtime.GC()
	}
	fmt.Printf("End %v 执行时长 : %.2fs \n", time.Now().Format("2006-01-02 15:04:05"), time.Since(startTime).Seconds())
}
