package main

import (
	"fmt"
	"iter"
	"maps"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/exp/constraints"
)

// Константы для распределения задач
const (
	numSimulations  = 10000 // Количество симуляций
	simulationYears = 3
	simulationDays  = 247 * simulationYears // Длительность одной симуляции (рабочих дней)

	developerCost      = 2000 * simulationYears
	developersMaxCount = 4

	initialTasksCount  = developersMaxCount * 2
	addTasksCount      = 2
	addTasksEveryXDays = 10

	TaskXXSMinDays = 1
	TaskXXSMaxDays = 5

	TaskXSMinDays = 6
	TaskXSMaxDays = 10

	TaskSMinDays = 11
	TaskSMaxDays = 20

	TaskMMinDays = 21
	TaskMMaxDays = 40

	TaskLMinDays = 41
	TaskLMaxDays = 80

	TaskXLMinDays = 81
	TaskXLMaxDays = 160

	LargeQueue QueueType = "Крупные"
	//MiddleQueue  QueueType = "Средние"
	//MiddleQueue2 QueueType = "Средние2"
	SmallQueue QueueType = "Малые"

	RiskSensitivity = 1.2
)

var (
	taskDistribution = map[TaskSize]float64{
		"xxs": 0.150, "xs": 0.476, "s": 0.206, "m": 0.106, "l": 0.041, "xl": 0.021, // ALL
		//"xxs": 0.172, "xs": 0.438, "s": 0.188, "m": 0.109, "l": 0.078, "xl": 0.016,
	}
	taskSizes = []TaskSize{"xxs", "xs", "s", "m", "l", "xl"}
	queues    = []QueueType{
		LargeQueue,
		//MiddleQueue,
		//MiddleQueue2,
		SmallQueue,
	}
	uncertaintyRatios = []float64{0.05, 0.50, 0.95}
)

type AvgTasksDone struct {
	AvgValue float64
	AvgCount float64
}

type DeveloperSummaryStats struct {
	AvgTasksDone   AvgTasksDone
	AvgDaysWaiting float64
}

type SimulationSummaryStats struct {
	Settings             Settings
	DeveloperCount       int
	TotalCosts           float64
	CostsPerQueue        map[QueueType]float64
	AvgTotalTasksDone    AvgTasksDone
	AvgTasksDonePerQueue map[QueueType]AvgTasksDone
	AvgQueueSize         map[QueueType]float64
	AvgTasksRest         map[TaskSize]float64
	AvgDevResults        map[string]DeveloperSummaryStats
}

func main() {
	rand.Seed(time.Now().UnixNano())
	startTime := time.Now()

	report := newXlsReport()
	defer report.Close()

	report.AddHeaders()

	confNum := 0

	for _, queueSplit := range distributeTaskSizes(queues, taskSizes) {
		for d := 1; d <= developersMaxCount; d++ {
			for _, developersPerQueue := range distributeDevelopers(queues, d) {
				if !isCorrectConfig(queues, queueSplit, developersPerQueue) {
					continue
				}
				for prioritizationStrategy := range prioritizationStrategies {
					for _, uncertaintyMaxRatio := range uncertaintyRatios {
						confNum++
						//eg.Go(func() error {
						settings := Settings{
							InitialTasksCount:               initialTasksCount,
							AddTasksCount:                   addTasksCount,
							AddTasksEveryXDays:              addTasksEveryXDays,
							SimulationDays:                  simulationDays,
							TaskGenerator:                   generateTask,
							QueueSplit:                      queueSplit,
							DevelopersPerQueue:              developersPerQueue,
							PrioritizationStrategy:          prioritizationStrategy,
							RequirementsUncertaintyMaxRatio: uncertaintyMaxRatio,
						}

						var results []SimulationResult
						// Запуск симуляций
						for i := 0; i < numSimulations; i++ {

							sim := NewSimulation(settings)
							sim.Run()

							results = append(results, sim.GetResult())

						}

						// Вывод итоговой статистики
						stats := calculateStats(settings, results)
						printSummaryStats(stats)
						report.AddSimulation(stats)
					}
				}
			}
		}
	}

	report.Save()

	fmt.Printf("\nПроверено %d конфигураций в %d симуляциях за %v\n", confNum, confNum*numSimulations, time.Since(startTime))
}

func calculateStats(settings Settings, results []SimulationResult) SimulationSummaryStats {

	sumTotalTasksDone := 0
	sumTotalTasksValue := 0
	sumTasksDonePerQueue := make(map[QueueType]TasksDone)
	sumQueueSize := make(map[QueueType]int)
	sumTasksRest := make(map[TaskSize]int)
	sumDevResults := make(map[string]struct {
		sumTasksValue, sumTasksDone, sumDaysWaiting, count int
	})
	for _, result := range results {
		sumTotalTasksValue += result.TotalTasksDone.Value
		sumTotalTasksDone += result.TotalTasksDone.Count
		for queueType, tasksDone := range result.TasksDonePerQueue {
			sumTasksDone := sumTasksDonePerQueue[queueType]
			sumTasksDone.Value += tasksDone.Value
			sumTasksDone.Count += tasksDone.Count
			sumTasksDonePerQueue[queueType] = sumTasksDone
		}
		for queueType, size := range result.QueueSize {
			sumQueueSize[queueType] += size
		}
		for size, count := range result.TasksRestPerSize {
			sumTasksRest[size] += count
		}
		for _, dev := range result.DevResults {
			key := dev.ID
			s := sumDevResults[key]
			s.sumTasksValue += dev.TotalTasksDone.Value
			s.sumTasksDone += dev.TotalTasksDone.Count
			s.sumDaysWaiting += dev.DaysWaiting
			s.count++
			sumDevResults[key] = s
		}
	}

	devCount := sum(maps.Values(settings.DevelopersPerQueue))

	return SimulationSummaryStats{
		Settings:       settings,
		DeveloperCount: devCount,
		TotalCosts:     float64(devCount) * developerCost,
		CostsPerQueue: MapFn(settings.DevelopersPerQueue, func(k QueueType, v int) float64 {
			return float64(v) * developerCost
		}),
		AvgTotalTasksDone: AvgTasksDone{
			AvgValue: float64(sumTotalTasksValue) / float64(len(results)),
			AvgCount: float64(sumTotalTasksDone) / float64(len(results)),
		},
		AvgTasksDonePerQueue: MapFn(sumTasksDonePerQueue, func(k QueueType, v TasksDone) AvgTasksDone {
			return AvgTasksDone{
				AvgValue: float64(v.Value) / float64(len(results)),
				AvgCount: float64(v.Count) / float64(len(results)),
			}
		}),
		AvgQueueSize: MapFn(sumQueueSize, func(k QueueType, v int) float64 {
			return float64(v) / float64(len(results))
		}),
		AvgTasksRest: MapFn(sumTasksRest, func(k TaskSize, v int) float64 {
			return float64(v) / float64(len(results))
		}),
		AvgDevResults: MapFn(sumDevResults, func(k string, v struct {
			sumTasksValue, sumTasksDone, sumDaysWaiting, count int
		}) DeveloperSummaryStats {
			return DeveloperSummaryStats{
				AvgTasksDone: AvgTasksDone{
					AvgValue: float64(v.sumTasksValue) / float64(v.count),
					AvgCount: float64(v.sumTasksDone) / float64(v.count),
				},
				AvgDaysWaiting: float64(v.sumDaysWaiting) / float64(v.count),
			}
		}),
	}
}

func printSummaryStats(stats SimulationSummaryStats) {
	fmt.Println(strings.Repeat("=", 90))
	fmt.Println("Очереди:")
	fmt.Printf("%-14s", "")
	for _, queue := range queues {
		fmt.Printf(" %-21s |", queue)
	}
	fmt.Println()

	fmt.Printf("%-14s", "Размеры:")
	for _, queue := range queues {
		fmt.Printf(" %-21s |", joinTaskSizes(stats.Settings.QueueSplit[queue]))
	}
	fmt.Println()

	fmt.Printf("%-14s", "Разработчиков:")
	for _, queue := range queues {
		fmt.Printf(" %21d |", stats.Settings.DevelopersPerQueue[queue])
	}
	fmt.Println()

	fmt.Printf("%-14s", "Сделано задач:")
	for _, queue := range queues {
		fmt.Printf(" %21.1f |", stats.AvgTasksDonePerQueue[queue].AvgCount)
	}
	fmt.Println()

	fmt.Printf("%-14s", "Бэклог:")
	for _, queue := range queues {
		fmt.Printf(" %21.1f |", stats.AvgQueueSize[queue])
	}
	fmt.Println()

	fmt.Printf("%-14s", "Доход:")
	for _, queue := range queues {
		fmt.Printf(" %21.1f |", stats.AvgTasksDonePerQueue[queue].AvgValue)
	}
	fmt.Println()

	fmt.Printf("%-14s", "Расход:")
	for _, queue := range queues {
		fmt.Printf(" %21.1f |", stats.CostsPerQueue[queue])
	}
	fmt.Println()

	fmt.Printf("%-14s", "Прибыль:")
	for _, queue := range queues {
		fmt.Printf(" %21.1f |", stats.AvgTasksDonePerQueue[queue].AvgValue-stats.CostsPerQueue[queue])
	}
	fmt.Println()

	fmt.Println(strings.Repeat("-", 90))

	fmt.Println("Средние показатели разработчиков:")
	fmt.Println("Разработчик | Приносит ценности в среднем | Делает задач в среднем | Ждет задачу в среднем")
	for id, dev := range stats.AvgDevResults {
		fmt.Printf("%-11s | %27.1f | %22.1f | %21.1f\n", id, dev.AvgTasksDone.AvgValue, dev.AvgTasksDone.AvgCount, dev.AvgDaysWaiting)
	}

	fmt.Println(strings.Repeat("-", 90))
	fmt.Println("Итого:")

	fmt.Printf("Доход: %.1f\n", stats.AvgTotalTasksDone.AvgValue)
	fmt.Printf("Расход: %.1f\n", stats.TotalCosts)
	fmt.Printf("Прибыль: %.1f\n", stats.AvgTotalTasksDone.AvgValue-stats.TotalCosts)
	fmt.Printf("Кол-во разработчиков: %d\n", stats.DeveloperCount)
	fmt.Printf("Приоритизация: %s\n", stats.Settings.PrioritizationStrategy)
	fmt.Printf("Степень неопределенности: %.3f\n", stats.Settings.RequirementsUncertaintyMaxRatio)
}

type xlsReport struct {
	f       *excelize.File
	nextRow int
	nextCol int
	getters map[int]func(stats SimulationSummaryStats) interface{}
}

func newXlsReport() *xlsReport {
	f := excelize.NewFile()
	//i, _ := f.NewSheet("Simulations")
	//f.SetActiveSheet(i)
	return &xlsReport{
		f:       f,
		nextRow: 1,
		nextCol: 0,
		getters: make(map[int]func(stats SimulationSummaryStats) interface{}),
	}
}

func (r *xlsReport) AddHeaders() {
	r.AddHeaderAndCellValueGetter("#", func(stats SimulationSummaryStats) interface{} {
		return r.nextRow - 1
	})

	r.AddHeaderAndCellValueGetter("Доход", func(stats SimulationSummaryStats) interface{} {
		return stats.AvgTotalTasksDone.AvgValue
	})
	r.AddHeaderAndCellValueGetter("Расход", func(stats SimulationSummaryStats) interface{} {
		return stats.TotalCosts
	})
	r.AddHeaderAndCellValueGetter("Прибыль", func(stats SimulationSummaryStats) interface{} {
		return stats.AvgTotalTasksDone.AvgValue - stats.TotalCosts
	})

	r.AddHeaderAndCellValueGetter("Степень неопределенности", func(stats SimulationSummaryStats) interface{} {
		return stats.Settings.RequirementsUncertaintyMaxRatio
	})

	r.AddHeaderAndCellValueGetter("Кол-во разработчиков", func(stats SimulationSummaryStats) interface{} {
		return stats.DeveloperCount
	})

	r.AddHeaderAndCellValueGetter("Приоритизация", func(stats SimulationSummaryStats) interface{} {
		return stats.Settings.PrioritizationStrategy
	})

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Размер", queue), func(stats SimulationSummaryStats) interface{} {
			return joinTaskSizes(stats.Settings.QueueSplit[queue])
		})
	}

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Разработчики", queue), func(stats SimulationSummaryStats) interface{} {
			return stats.Settings.DevelopersPerQueue[queue]
		})
	}

	r.AddHeaderAndCellValueGetter("Сделано задач", func(stats SimulationSummaryStats) interface{} {
		var sum float64
		for _, queue := range queues {
			sum += stats.AvgTasksDonePerQueue[queue].AvgCount
		}
		return sum
	})

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Сделано задач", queue), func(stats SimulationSummaryStats) interface{} {
			return stats.AvgTasksDonePerQueue[queue].AvgCount
		})
	}

	r.AddHeaderAndCellValueGetter("Бэклог", func(stats SimulationSummaryStats) interface{} {
		var sum float64
		for _, queue := range queues {
			sum += stats.AvgQueueSize[queue]
		}
		return sum
	})

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Бэклог", queue), func(stats SimulationSummaryStats) interface{} {
			return stats.AvgQueueSize[queue]
		})
	}

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Доход", queue), func(stats SimulationSummaryStats) interface{} {
			return stats.AvgTasksDonePerQueue[queue].AvgValue
		})
	}

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Расход", queue), func(stats SimulationSummaryStats) interface{} {
			return stats.CostsPerQueue[queue]
		})
	}

	for _, queue := range queues {
		queue := queue
		r.AddHeaderAndCellValueGetter(fmt.Sprintf("%s (%s)", "Прибыль", queue), func(stats SimulationSummaryStats) interface{} {
			return stats.AvgTasksDonePerQueue[queue].AvgValue - stats.CostsPerQueue[queue]
		})
	}

	r.nextRow++
}

func (r *xlsReport) AddHeaderAndCellValueGetter(header string, fn func(stats SimulationSummaryStats) interface{}) {
	r.f.SetCellValue("Sheet1", cellName(r.nextRow, r.nextCol), header)
	r.getters[r.nextCol] = fn
	r.nextCol++
}

func (r *xlsReport) AddSimulation(stats SimulationSummaryStats) {
	for i := 0; i < r.nextCol; i++ {
		if getter, ok := r.getters[i]; ok {
			r.f.SetCellValue("Sheet1", cellName(r.nextRow, i), getter(stats))
		}
	}
	r.nextRow++
}

func (r *xlsReport) Save() {
	filename := fmt.Sprintf("simulation_results_%s.xlsx", time.Now().Format("2006-01-02_15-04-05"))
	r.f.SaveAs(filename)
	fmt.Printf("Результаты сохранены в файл: %s\n", filename)
}

func (r *xlsReport) Close() {
	r.f.Close()
}

func cellName(row, col int) string {
	return fmt.Sprintf("%c%d", rune('A'+col), row)
}

func generateTask(requirementsUncertaintyMaxRatio float64) Task {
	size := selectTaskSize()
	var duration, value int

	switch size {
	case "xxs":
		duration = rand.Intn(TaskXXSMaxDays-TaskXXSMinDays+1) + TaskXXSMinDays
		value = rand.Intn(100) + 1
	case "xs":
		duration = rand.Intn(TaskXSMaxDays-TaskXSMinDays+1) + TaskXSMinDays
		value = rand.Intn(200) + 50
	case "s":
		duration = rand.Intn(TaskSMaxDays-TaskSMinDays+1) + TaskSMinDays
		value = rand.Intn(300) + 150
	case "m":
		duration = rand.Intn(TaskMMaxDays-TaskMMinDays+1) + TaskMMinDays
		value = rand.Intn(400) + 300
	case "l":
		duration = rand.Intn(TaskLMaxDays-TaskLMinDays+1) + TaskLMinDays
		value = rand.Intn(300) + 500
	case "xl":
		duration = rand.Intn(TaskXLMaxDays-TaskXLMinDays+1) + TaskXLMinDays
		value = rand.Intn(200) + 700
	}

	value = int(float64(value) * (0.8 + 0.4*rand.Float64()))
	if value < 1 {
		value = 1
	}

	return Task{
		ID:                  rand.Intn(1000000),
		Size:                size,
		Duration:            duration,
		Value:               value,
		RequirementsClarity: 1.0 - requirementsUncertaintyMaxRatio*rand.Float64(),
	}
}

func selectTaskSize() TaskSize {
	r := rand.Float64()
	cumulative := 0.0
	for size, prob := range taskDistribution {
		cumulative += prob
		if r <= cumulative {
			return size
		}
	}
	return "s"
}

func distributeDevelopers(queues []QueueType, n int) []map[QueueType]int {
	var results []map[QueueType]int
	stack := []struct {
		distribution map[QueueType]int
		remaining    int
		queueIndex   int
	}{
		{make(map[QueueType]int), n, 0},
	}

	for len(stack) > 0 {
		// Извлекаем текущее состояние из стека
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Если распределили все корзины
		if current.queueIndex == len(queues) {
			if current.remaining == 0 {
				results = append(results, current.distribution)
			}
			continue
		}

		queueName := queues[current.queueIndex]
		// Распределяем от 0 до всех оставшихся яблок в текущую корзину
		for developers := 0; developers <= current.remaining; developers++ {
			// Создаем копию текущего распределения
			newDist := make(map[QueueType]int)
			for k, v := range current.distribution {
				newDist[k] = v
			}
			newDist[queueName] = developers

			// Добавляем новое состояние в стек
			stack = append(stack, struct {
				distribution map[QueueType]int
				remaining    int
				queueIndex   int
			}{newDist, current.remaining - developers, current.queueIndex + 1})
		}
	}

	return results
}

func distributeTaskSizes(queues []QueueType, sizes []TaskSize) []map[QueueType][]TaskSize {
	results := make([]map[QueueType][]TaskSize, 0)
	if len(queues) == 0 {
		return results
	}
	queue := queues[0]
	if len(queues) == 1 {
		return append(results, map[QueueType][]TaskSize{queue: sizes})
	}
	for i := range sizes {
		splits := distributeTaskSizes(queues[1:], sizes[0:i])
		if len(splits) == 0 {
			results = append(results, map[QueueType][]TaskSize{queue: sizes[i:]})
		}
		for _, split := range splits {
			split[queue] = sizes[i:]
			results = append(results, split)
		}
	}
	return results
}

func joinTaskSizes(sizes []TaskSize) string {
	s := make([]TaskSize, len(sizes))
	copy(s, sizes)
	slices.Reverse(s)
	var sb strings.Builder
	for i, size := range s {
		sb.WriteString(string(size))
		if i < len(sizes)-1 {
			sb.WriteString(", ")
		}
	}
	return sb.String()
}

func MapFn[K comparable, V, R any](m map[K]V, fn func(K, V) R) map[K]R {
	if m == nil {
		return nil
	}
	r := make(map[K]R, len(m))
	for key, value := range m {
		r[key] = fn(key, value)
	}
	return r
}

func sum[V constraints.Integer | constraints.Float](a iter.Seq[V]) V {
	var sum V
	for v := range a {
		sum += v
	}
	return sum
}

func isCorrectConfig(queues []QueueType, queueSplit map[QueueType][]TaskSize, developersPerQueue map[QueueType]int) bool {
	for _, queue := range queues {
		if len(queueSplit[queue]) == 0 && developersPerQueue[queue] > 0 {
			return false
		}
	}
	return true
}
