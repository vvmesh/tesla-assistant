package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/go-co-op/gocron/v2"
)

var opts = &slog.HandlerOptions{
	Level: slog.LevelDebug,
}
var logger = slog.New(slog.NewTextHandler(os.Stdout, opts))

var latestState ResposneStatus

var config Configuration

type ResposneStatus struct {
	Data struct {
		Car struct {
			CarId   int    `json:"car_id"`
			CarName string `json:"car_name"`
		}

		Status struct {
			ChargingDetails struct {
				ChargeCurrentRequest       int     `json:"charge_current_request"`
				ChargeCurrentRequestMax    int     `json:"charge_current_request_max"`
				ChargeEnergyAdded          float32 `json:"charge_energy_added"`
				ChargeLimitSoc             int     `json:"charge_limit_soc"`
				ChargePortDoorOpen         bool    `json:"charge_port_door_open"`
				ChargerActualCurrent       int     `json:"charger_actual_current"`
				ChargerPhases              int     `json:"charger_phases"`
				ChargerPower               int     `json:"charger_power"`
				ChargerVoltage             int     `json:"charger_voltage"`
				PluggedIn                  bool    `json:"plugged_in"`
				ScheduledChargingStartTime string  `json:"scheduled_charging_start_time"`
				TimeToFullCharge           float32 `json:"time_to_full_charge"`
			} `json:"charging_details"`
			BatteryDetails struct {
				BatteryLevel       int     `json:"battery_level"`
				EstBatteryRange    float32 `json:"est_battery_range"`
				IdealBatteryRange  float32 `json:"ideal_battery_range"`
				RatedBatteryRange  float32 `json:"rated_battery_range"`
				UsableBatteryLevel int     `json:"usable_battery_level"`
			} `json:"battery_details"`

			ClimateDetails struct {
				InsideTemp        float32 `json:"inside_temp"`
				IsClimateOn       bool    `json:"is_climate_on"`
				IsPreconditioning bool    `json:"is_preconditioning"`
				OutsideTemp       float32 `json:"outside_temp"`
			} `json:"climate_details"`

			State      string `json:"state"`
			StateSince string `json:"state_since"`
		}
	}
}

type Configuration struct {
	TeslaApiURL      string
	DingRobotWebhook string
}

func loadConfig() Configuration {
	return Configuration{
		TeslaApiURL:      os.Getenv("TESLA_API_URL"),            // https://your_teslamate_api_server/api
		DingRobotWebhook: os.Getenv("NOTIFY_DINGROBOT_WEBHOOK"), // https://oapi.dingtalk.com/robot/send?access_token=your_access_token
	}
}

func main() {
	config = loadConfig()

	//run task on start
	cronTask()

	// create a scheduler
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		logger.Error("Create scheduler error:", err)
		return
	}

	// add a job to the scheduler
	j, err := scheduler.NewJob(
		gocron.DurationJob(
			60*time.Second,
		),

		gocron.NewTask(
			func(a string, b int) {
				cronTask()
			},
			"cronTask",
			1,
		),
	)

	if err != nil {
		logger.Error("Create job error:", err)
		return
	}
	// each job has a unique id
	logger.Error("Create job : ", j.ID())

	// start the scheduler
	scheduler.Start()

	// block until you are ready to shut down
	select {
	//case <-time.After(10 * time.Second):
	}

	// when you're done, shut it down
	//err = s.Shutdown()
	//if err != nil {
	//      logger.Error("Shutdown scheduler error:", err)
	//}

}
func cronTask() {
	// 执行你的任务代码
	logger.Debug("Task executed at", time.Now())

	response, err := requestTeslaAPI()
	if err != nil {
		logger.Error("failed to call api: ", err)
		return
	}

	logger.Debug(fmt.Sprintf("CarId: %d", response.Data.Car.CarId))
	logger.Debug(fmt.Sprintf("CarName: %s", response.Data.Car.CarName))

	//车辆状态 charging|sleep
	logger.Debug(fmt.Sprintf("State: %s", response.Data.Status.State))

	t, err := time.Parse(time.RFC3339, response.Data.Status.StateSince)

	logger.Debug(fmt.Sprintf("StateSince: %s", t))

	//剩余电量   xx (%)   如 80
	logger.Debug(fmt.Sprintf("BatteryLevel: %d", response.Data.Status.BatteryDetails.BatteryLevel))

	// 读取电源插头状态 true|false
	logger.Debug(fmt.Sprintf("PluggedIn: %t", response.Data.Status.ChargingDetails.PluggedIn))

	// 读取充电状态 32  充电器最大功率？？？？
	logger.Debug(fmt.Sprintf("ChargerActualCurrent: %d", response.Data.Status.ChargingDetails.ChargerActualCurrent))

	//充满剩余时间 x.xx(小时)
	logger.Debug(fmt.Sprintf("TimeToFullCharge: %f", response.Data.Status.ChargingDetails.TimeToFullCharge))

	//充电功率 7 16 32  (kw)
	logger.Debug(fmt.Sprintf("ChargerPower: %d", response.Data.Status.ChargingDetails.ChargerPower))

	//充电阶段 1-交流？充电中？？？
	logger.Debug(fmt.Sprintf("ChargerPhases: %d", response.Data.Status.ChargingDetails.ChargerPhases))

	inspect(response)

	//更新最新数据状态
	latestState = response

}

func requestTeslaAPI() (ResposneStatus, error) {

	// 检查车辆状态
	response, err := http.Get(config.TeslaApiURL + "/v1/cars/1/status")
	if err != nil {
		return ResposneStatus{}, fmt.Errorf("failed to send request: %v", err)
	}
	defer response.Body.Close()

	// 读取响应内容
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return ResposneStatus{}, fmt.Errorf("failed to read response: %v", err)
	}

	jsonString := string(body)

	// 输出响应内容
	//printJson(jsonString)
	data := &ResposneStatus{}
	parse_err := json.Unmarshal([]byte(jsonString), data)
	if parse_err != nil {
		return ResposneStatus{}, fmt.Errorf("failed to parse response: %v", parse_err)
	}
	return *data, nil

}

func inspect(currentState ResposneStatus) error {

	if reflect.ValueOf(latestState).IsZero() {
		notify("监控程序启动", currentState)
	} else {
		//由非充电状态 进入充电状态
		if !latestState.Data.Status.ChargingDetails.PluggedIn && currentState.Data.Status.ChargingDetails.PluggedIn {
			notify("充电枪已接入", currentState)
		}
		if "charging" != latestState.Data.Status.State && "charging" == currentState.Data.Status.State {
			notify("已开始充电", currentState)
		}

		if "charging" == latestState.Data.Status.State && "charging" != currentState.Data.Status.State {
			notify("已停止充电", currentState)
		}
		if latestState.Data.Status.ChargingDetails.PluggedIn && !currentState.Data.Status.ChargingDetails.PluggedIn {
			notify("充电枪已断开", currentState)
		}

	}

	//检查状态
	if currentState.Data.Status.ChargingDetails.PluggedIn {

		var timeToFullChargeSeconds = int(3600 * currentState.Data.Status.ChargingDetails.TimeToFullCharge)

		//已满电
		if timeToFullChargeSeconds <= 0 {
			notify("已完成充电", currentState)
		} else if timeToFullChargeSeconds == 300 {
			notify("即将完成充电", currentState)
		}
	}

	return nil

}

func notify(title string, currentState ResposneStatus) error {

	content := fmt.Sprintf("## Tesla%s  \n  ", title)
	content += fmt.Sprintf("#### - 车辆名称\t%s  \n  ", currentState.Data.Car.CarName)
	content += fmt.Sprintf("#### - 车辆状态\t%s  \n  ", currentState.Data.Status.State)
	content += fmt.Sprintf("#### - 电池电量\t%d%%  \n  ", currentState.Data.Status.BatteryDetails.BatteryLevel)
	if currentState.Data.Status.ChargingDetails.PluggedIn {
		content += fmt.Sprintf("#### - 充电枪接入\t%t  \n  ", currentState.Data.Status.ChargingDetails.PluggedIn)
	}
	if currentState.Data.Status.ChargingDetails.ChargerActualCurrent != 0 {
		content += fmt.Sprintf("#### - 充电功率\t%dkw  \n  ", currentState.Data.Status.ChargingDetails.ChargerActualCurrent)
	}
	if currentState.Data.Status.ChargingDetails.TimeToFullCharge != 0 {
		content += fmt.Sprintf("#### - 剩余时间\t%d分钟  \n  ", int(60*currentState.Data.Status.ChargingDetails.TimeToFullCharge))
	}
	stateSince := currentState.Data.Status.StateSince
	stateSinceTime, err := time.Parse(time.RFC3339, currentState.Data.Status.StateSince)
	if err == nil {
		stateSince = stateSinceTime.Format("2006-01-02 15:04:05")
	}
	content += fmt.Sprintf("#### - 状态更新\t%s  \n  ", stateSince)
	content += fmt.Sprintf("###### %s \n  ", time.Now().Format("2006-01-02 15:04:05"))

	sendDingTalkMessage(title, content)
	if err != nil {
		return fmt.Errorf("Send DingTalk message failed: %v", err)
	}
	return nil

}

type Message struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"markdown"`
}

func sendDingTalkMessage(title string, text string) error {

	// 创建消息
	msg := Message{
		MsgType: "markdown",
	}
	msg.Markdown.Title = title
	msg.Markdown.Text = text

	// 将消息编码为 JSON
	msgData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	// 发送 POST 请求
	resp, err := http.Post(config.DingRobotWebhook, "application/json", bytes.NewBuffer(msgData))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to response code, status: %v", err)
	}
	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	jsonString := string(body)

	// 输出响应内容
	logger.Debug(jsonString)
	return nil
}

/*
func printJson(jsonStr string) {
        var data interface{}
        err := json.Unmarshal([]byte(jsonStr), &data)
        if err != nil {
                logger.Error("解析 JSON 错误:", err)
                return
        }

        formattedJSON, err := json.MarshalIndent(data, "", "  ")
        if err != nil {
                logger.Error("格式化 JSON 错误")
                return
        }
        fmt.Println(string(formattedJSON))
}
*/
