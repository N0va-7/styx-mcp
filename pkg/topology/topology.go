package topology

import (
	"fmt"
	"strings"

	"mcp-stowaway/internal/utils"
	"mcp-stowaway/pkg/protocol"
)

// Task modes for Topology.
const (
	AddNode = iota
	GetUUID
	GetUUIDNum
	CheckNode
	Calculate
	GetRoute
	DelNode
	ReonlineNode
	GetNode
	UpdateDetail
	ShowDetail
	ShowTopo
	UpdateMemo
	GetParent
	GetFirstHop
)

// Node represents an online agent in the topology.
type Node struct {
	UUID            string
	ParentUUID      string
	ChildrenUUID    []string
	CurrentUser     string
	CurrentHostname string
	CurrentIP       string
	Memo            string
}

// NewNode creates a topology node.
func NewNode(uuid, ip string) *Node {
	return &Node{
		UUID:   uuid,
		CurrentIP: ip,
	}
}

// Topology maintains the tree of online nodes.
type Topology struct {
	currentIDNum int
	nodes        map[int]*Node
	routes       map[string]string
	history      map[string]int

	TaskChan   chan *Task
	ResultChan chan *Result
}

// Task is a request sent to the topology manager.
type Task struct {
	Mode       int
	UUID       string
	UUIDNum    int
	ParentUUID string
	Target     *Node
	HostName   string
	UserName   string
	Memo       string
	IsFirst    bool
}

// Result is returned by the topology manager.
type Result struct {
	IsExist  bool
	UUID     string
	Route    string
	IDNum    int
	AllNodes []string
	Node     *Node
}

// NewTopology creates a new topology manager.
func NewTopology() *Topology {
	return &Topology{
		nodes:      make(map[int]*Node),
		routes:     make(map[string]string),
		history:    make(map[string]int),
		TaskChan:   make(chan *Task),
		ResultChan: make(chan *Result),
	}
}

// Run starts the topology event loop.
func (t *Topology) Run() {
	for task := range t.TaskChan {
		switch task.Mode {
		case AddNode:
			t.addNode(task)
		case GetUUID:
			t.getUUID(task)
		case GetUUIDNum:
			t.getUUIDNum(task)
		case CheckNode:
			t.checkNode(task)
		case Calculate:
			t.calculate()
		case GetRoute:
			t.getRoute(task)
		case GetFirstHop:
			t.getFirstHop(task)
		case DelNode:
			t.delNode(task)
		case ReonlineNode:
			t.reonlineNode(task)
		case GetNode:
			t.getNode(task)
		case UpdateDetail:
			t.updateDetail(task)
		case ShowDetail:
			t.showDetail()
		case ShowTopo:
			t.showTopo()
		case UpdateMemo:
			t.updateMemo(task)
		case GetParent:
			t.getParent(task)
		}
	}
}

func (t *Topology) id2Num(uuid string) int {
	for num, node := range t.nodes {
		if node.UUID == uuid {
			return num
		}
	}
	return -1
}

func (t *Topology) num2ID(num int) (string, bool) {
	node, ok := t.nodes[num]
	if !ok {
		return "", false
	}
	return node.UUID, true
}

func (t *Topology) getUUID(task *Task) {
	uuid, ok := t.num2ID(task.UUIDNum)
	if !ok {
		t.ResultChan <- &Result{UUID: ""}
		return
	}
	t.ResultChan <- &Result{UUID: uuid}
}

func (t *Topology) getUUIDNum(task *Task) {
	t.ResultChan <- &Result{IDNum: t.id2Num(task.UUID)}
}

func (t *Topology) getNode(task *Task) {
	num := t.id2Num(task.UUID)
	if num < 0 {
		t.ResultChan <- &Result{IsExist: false}
		return
	}
	node := t.nodes[num]
	copy := *node
	copy.ChildrenUUID = append([]string{}, node.ChildrenUUID...)
	t.ResultChan <- &Result{IsExist: true, Node: &copy}
}

func (t *Topology) checkNode(task *Task) {
	_, ok := t.nodes[task.UUIDNum]
	t.ResultChan <- &Result{IsExist: ok}
}

func (t *Topology) addNode(task *Task) {
	if task.IsFirst {
		task.Target.ParentUUID = protocol.ADMIN_UUID
	} else {
		task.Target.ParentUUID = task.ParentUUID
		parentNum := t.id2Num(task.ParentUUID)
		if parentNum < 0 {
			t.ResultChan <- &Result{IDNum: -1}
			return
		}
		t.nodes[parentNum].ChildrenUUID = append(t.nodes[parentNum].ChildrenUUID, task.Target.UUID)
	}

	t.nodes[t.currentIDNum] = task.Target
	t.history[task.Target.UUID] = t.currentIDNum
	t.ResultChan <- &Result{IDNum: t.currentIDNum}
	t.currentIDNum++
}

func (t *Topology) calculate() {
	newRoutes := make(map[string]string)

	for num := range t.nodes {
		node := t.nodes[num]
		if node.ParentUUID == protocol.ADMIN_UUID {
			newRoutes[node.UUID] = ""
			continue
		}

		var route []string
		currentNum := num
		for {
			if t.nodes[currentNum].ParentUUID == protocol.ADMIN_UUID {
				utils.StringSliceReverse(route)
				newRoutes[node.UUID] = strings.Join(route, ":")
				break
			}
			route = append(route, t.nodes[currentNum].UUID)
			parentNum := t.id2Num(t.nodes[currentNum].ParentUUID)
			if parentNum < 0 {
				newRoutes[node.UUID] = ""
				break
			}
			currentNum = parentNum
		}
	}

	t.routes = newRoutes
	t.ResultChan <- &Result{}
}

func (t *Topology) getRoute(task *Task) {
	t.ResultChan <- &Result{Route: t.routes[task.UUID]}
}

func (t *Topology) updateDetail(task *Task) {
	num := t.id2Num(task.UUID)
	if num < 0 {
		t.ResultChan <- &Result{}
		return
	}
	t.nodes[num].CurrentUser = task.UserName
	t.nodes[num].CurrentHostname = task.HostName
	t.nodes[num].Memo = task.Memo
	t.ResultChan <- &Result{}
}

func (t *Topology) updateMemo(task *Task) {
	num := t.id2Num(task.UUID)
	if num >= 0 {
		t.nodes[num].Memo = task.Memo
	}
	t.ResultChan <- &Result{}
}

func (t *Topology) getParent(task *Task) {
	num := t.id2Num(task.UUID)
	if num < 0 {
		t.ResultChan <- &Result{UUID: ""}
		return
	}
	t.ResultChan <- &Result{UUID: t.nodes[num].ParentUUID}
}

func (t *Topology) getFirstHop(task *Task) {
	num := t.id2Num(task.UUID)
	if num < 0 {
		t.ResultChan <- &Result{}
		return
	}

	// Walk up to the node directly connected to admin.
	var routeParts []string
	current := num
	for {
		node := t.nodes[current]
		if node.ParentUUID == protocol.ADMIN_UUID {
			t.ResultChan <- &Result{UUID: node.UUID, Route: strings.Join(routeParts, ":")}
			return
		}
		routeParts = append([]string{node.UUID}, routeParts...)
		parentNum := t.id2Num(node.ParentUUID)
		if parentNum < 0 {
			t.ResultChan <- &Result{}
			return
		}
		current = parentNum
	}
}

func (t *Topology) showDetail() {
	for num, node := range t.nodes {
		fmt.Printf("Node[%d] -> IP: %s  Hostname: %s  User: %s\nMemo: %s\n",
			num, node.CurrentIP, node.CurrentHostname, node.CurrentUser, node.Memo)
	}
	t.ResultChan <- &Result{}
}

func (t *Topology) showTopo() {
	for num, node := range t.nodes {
		fmt.Printf("Node[%d]'s children ->\n", num)
		for _, child := range node.ChildrenUUID {
			fmt.Printf("Node[%d]\n", t.id2Num(child))
		}
	}
	t.ResultChan <- &Result{}
}

func (t *Topology) delNode(task *Task) {
	idNum := t.id2Num(task.UUID)
	if idNum < 0 {
		t.ResultChan <- &Result{AllNodes: []string{}}
		return
	}

	var ready []int
	var readyUUID []string

	parentNum := t.id2Num(t.nodes[idNum].ParentUUID)
	if parentNum >= 0 {
		children := t.nodes[parentNum].ChildrenUUID
		for i, childUUID := range children {
			if childUUID == task.UUID {
				if i == len(children)-1 {
					t.nodes[parentNum].ChildrenUUID = children[:i]
				} else {
					t.nodes[parentNum].ChildrenUUID = append(children[:i], children[i+1:]...)
				}
				break
			}
		}
	}

	t.findChildren(&ready, idNum)
	ready = append(ready, idNum)

	for _, num := range ready {
		if uuid, ok := t.num2ID(num); ok {
			readyUUID = append(readyUUID, uuid)
		}
		delete(t.nodes, num)
	}

	t.ResultChan <- &Result{AllNodes: readyUUID}
}

func (t *Topology) findChildren(ready *[]int, idNum int) {
	for _, uuid := range t.nodes[idNum].ChildrenUUID {
		num := t.id2Num(uuid)
		if num >= 0 {
			*ready = append(*ready, num)
			t.findChildren(ready, num)
		}
	}
}

func (t *Topology) reonlineNode(task *Task) {
	if task.IsFirst {
		task.Target.ParentUUID = protocol.ADMIN_UUID
	} else {
		task.Target.ParentUUID = task.ParentUUID
		parentNum := t.id2Num(task.ParentUUID)
		if parentNum >= 0 {
			t.nodes[parentNum].ChildrenUUID = append(t.nodes[parentNum].ChildrenUUID, task.Target.UUID)
		}
	}

	var idNum int
	if num, ok := t.history[task.Target.UUID]; ok {
		idNum = num
	} else {
		idNum = t.currentIDNum
		t.history[task.Target.UUID] = idNum
		t.currentIDNum++
	}

	t.nodes[idNum] = task.Target
	t.ResultChan <- &Result{}
}

