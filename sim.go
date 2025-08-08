package main

import (
	"fmt"
	"math"
	"sort"
)

type QueueType string

type TaskSize string

type Task struct {
	ID                  int
	Size                TaskSize
	Duration            int
	Value               int
	RequirementsClarity float64 // 0.0 - 1.0
}

type Developer struct {
	ID                    string
	QueueType             QueueType
	CurrentTask           *Task
	CurrentTaskDaysWorked int
	TotalTasksDone        TasksDone
	TasksDonePerSize      map[TaskSize]TasksDone
	DaysWaiting           int
}

type TasksDone struct {
	Value int
	Count int
}

func (t *TasksDone) GetAvgValue() float64 {
	return float64(t.Value) / float64(t.Count)
}

func (t *Task) DurationWithRisks() float64 {
	return float64(t.Duration) + math.Exp(RiskSensitivity*(1.0-t.RequirementsClarity))
}

type Simulation struct {
	Tasks             map[QueueType][]Task
	Developers        []Developer
	TotalTasksDone    TasksDone
	TasksDonePerQueue map[QueueType]TasksDone
	Settings          Settings
}

type Settings struct {
	DevelopersPerQueue              map[QueueType]int
	QueueSplit                      map[QueueType][]TaskSize
	InitialTasksCount               int
	AddTasksCount                   int
	AddTasksEveryXDays              int
	SimulationDays                  int
	TaskGenerator                   func(float64) Task
	PrioritizationStrategy          string
	RequirementsUncertaintyMaxRatio float64 // максимальная степень неопределенности задач
}

type SimulationResult struct {
	TotalTasksDone    TasksDone               // Всего решено задач
	TasksDonePerQueue map[QueueType]TasksDone // Всего решено задач в очереди
	QueueSize         map[QueueType]int       // Бэклог
	TasksRestPerSize  map[TaskSize]int        // Бэклог по размерам задач
	DevResults        []Developer
}

func NewSimulation(settings Settings) *Simulation {
	sim := &Simulation{
		Settings:          settings,
		Tasks:             make(map[QueueType][]Task),
		TasksDonePerQueue: make(map[QueueType]TasksDone),
	}

	// Инициализируем разработчиков
	for queueType, count := range settings.DevelopersPerQueue {
		for i := 1; i <= count; i++ {
			sim.Developers = append(sim.Developers, Developer{
				ID:               fmt.Sprintf("%s%d", queueType, i),
				QueueType:        queueType,
				TasksDonePerSize: make(map[TaskSize]TasksDone),
			})
		}
	}

	return sim
}

func (sim *Simulation) Run() {
	sim.addTasks(sim.Settings.InitialTasksCount)
	for day := 0; day < sim.Settings.SimulationDays; day++ {

		if day%sim.Settings.AddTasksEveryXDays == 0 {
			sim.addTasks(sim.Settings.AddTasksCount)
		}

		for i := range sim.Developers {
			dev := &sim.Developers[i]
			tasks := sim.Tasks[dev.QueueType]

			if dev.CurrentTask == nil && len(tasks) > 0 {
				dev.CurrentTask = &tasks[0]
				sim.Tasks[dev.QueueType] = tasks[1:]
				dev.CurrentTaskDaysWorked = 0
			}

			if dev.CurrentTask != nil {
				dev.CurrentTaskDaysWorked++
				if dev.CurrentTaskDaysWorked >= int(dev.CurrentTask.DurationWithRisks()) {
					dev.TotalTasksDone.Value += dev.CurrentTask.Value
					dev.TotalTasksDone.Count++

					tasksDonePerSize := dev.TasksDonePerSize[dev.CurrentTask.Size]
					tasksDonePerSize.Value += dev.CurrentTask.Value
					tasksDonePerSize.Count++
					dev.TasksDonePerSize[dev.CurrentTask.Size] = tasksDonePerSize

					sim.TotalTasksDone.Value += dev.CurrentTask.Value
					sim.TotalTasksDone.Count++

					tasksDonePerQueue := sim.TasksDonePerQueue[dev.QueueType]
					tasksDonePerQueue.Value += dev.CurrentTask.Value
					tasksDonePerQueue.Count++
					sim.TasksDonePerQueue[dev.QueueType] = tasksDonePerQueue

					dev.CurrentTask = nil
				}
			} else {
				dev.DaysWaiting++
			}
		}
	}
}

func (sim *Simulation) GetResult() SimulationResult {
	devResults := make([]Developer, len(sim.Developers))
	copy(devResults, sim.Developers)

	rest := make(map[TaskSize]int)
	queueSize := make(map[QueueType]int)
	for queueType, tasks := range sim.Tasks {
		queueSize[queueType] = len(tasks)
		for _, task := range tasks {
			rest[task.Size]++
		}
	}

	return SimulationResult{
		TotalTasksDone:    sim.TotalTasksDone,
		TasksDonePerQueue: sim.TasksDonePerQueue,
		DevResults:        devResults,
		QueueSize:         queueSize,
		TasksRestPerSize:  rest,
	}
}

func (sim *Simulation) addTasks(count int) {
	for i := 0; i < count; i++ {
		task := sim.Settings.TaskGenerator(sim.Settings.RequirementsUncertaintyMaxRatio)
		for q, sizes := range sim.Settings.QueueSplit {
			for _, s := range sizes {
				if task.Size == s {
					sim.Tasks[q] = append(sim.Tasks[q], task)
					break
				}
			}
		}
	}

	for q := range sim.Tasks {
		if prioritizationStrategies[sim.Settings.PrioritizationStrategy] != nil {
			prioritizationStrategies[sim.Settings.PrioritizationStrategy](sim.Tasks[q])
		}
	}
}

// SortFunctions содержит все возможные способы сортировки
var prioritizationStrategies = map[string]func([]Task){
	"Нет":                         noSort,
	"Сначала быстрые":             sortByDurationAsc,
	"Сначала долгие":              sortByDurationDesc,
	"Сначала менее ценные":        sortByValueAsc,
	"Сначала более ценные":        sortByValueDesc,
	"Сначала менее проработанные": sortByClarityAsc,
	"Сначала более проработанные": sortByClarityDesc,
	"Сначала менее ценные (ценность/длительность)":           sortByValueDurationRatioAsc,
	"Сначала более ценные (ценность/длительность)":           sortByValueDurationRatioDesc,
	"Сначала менее ценные (ценность/длительность с рисками)": sortByValueDurationClarityRatioAsc,
	"Сначала более ценные (ценность/длительность с рисками)": sortByValueDurationClarityRatioDesc,
}

// Реализации всех функций сортировки

func noSort(_ []Task) {
	// Ничего не делаем - оставляем как есть
}

func sortByDurationAsc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Duration < tasks[j].Duration
	})
}

func sortByDurationDesc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Duration > tasks[j].Duration
	})
}

func sortByValueAsc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Value < tasks[j].Value
	})
}

func sortByValueDesc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Value > tasks[j].Value
	})
}

func sortByClarityAsc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].RequirementsClarity < tasks[j].RequirementsClarity
	})
}

func sortByClarityDesc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].RequirementsClarity > tasks[j].RequirementsClarity
	})
}

func sortByValueDurationRatioAsc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		ratioI := float64(tasks[i].Value) / float64(tasks[i].Duration)
		ratioJ := float64(tasks[j].Value) / float64(tasks[j].Duration)
		return ratioI < ratioJ
	})
}

func sortByValueDurationRatioDesc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		ratioI := float64(tasks[i].Value) / float64(tasks[i].Duration)
		ratioJ := float64(tasks[j].Value) / float64(tasks[j].Duration)
		return ratioI > ratioJ
	})
}

func sortByValueDurationClarityRatioAsc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		ratioI := float64(tasks[i].Value) / tasks[i].DurationWithRisks()
		ratioJ := float64(tasks[j].Value) / tasks[j].DurationWithRisks()
		return ratioI < ratioJ
	})
}

func sortByValueDurationClarityRatioDesc(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		ratioI := float64(tasks[i].Value) / tasks[i].DurationWithRisks()
		ratioJ := float64(tasks[j].Value) / tasks[j].DurationWithRisks()
		return ratioI > ratioJ
	})
}
