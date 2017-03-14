package log

import "bytes"

func ExampleLogger() {
	logger := Logger{}
	logger.Info("Info level log message", "key", "value")
	//Output: {"key":"value","level":"info","msg":"Info level log message"}
}

func ExampleLogger_context() {
	logger := Logger{}.With("key", "value")
	logger.Info("Info level log message")
	//Output: {"key":"value","level":"info","msg":"Info level log message"}
}

func ExampleLogger_logfmt() {
	logger := Logger{Formatter: LogfmtFormatter{}}
	logger.Info("Info level log message", "key", "value")
	//Output: level=info msg="Info level log message" key=value
}

func ExampleLogger_valueGenerator() {
	var v ValueGenerator = func() interface{} {
		return "generated"
	}
	logger := Logger{}.With("key", v)
	logger.Info("Info level log message")
	//Output: {"key":"generated","level":"info","msg":"Info level log message"}
}

func ExampleLogger_writer() {
	buffer := &bytes.Buffer{}
	logger := Logger{Writer: buffer}
	logger.Info("Info level log message", "key", "value")
	//Output:
}

func ExampleLogger_Alert() {
	logger := Logger{}
	logger.Alert("Alert level log message", "key", "value")
	//Output: {"key":"value","level":"alert","msg":"Alert level log message"}
}

func ExampleLogger_Critical() {
	logger := Logger{}
	logger.Critical("Critical level log message", "key", "value")
	//Output: {"key":"value","level":"critical","msg":"Critical level log message"}
}

func ExampleLogger_Debug() {
	logger := Logger{}
	logger.Debug("Debug level log message", "key", "value")
	//Output: {"key":"value","level":"debug","msg":"Debug level log message"}
}

func ExampleLogger_Emergency() {
	logger := Logger{}
	logger.Emergency("Emergency level log message", "key", "value")
	//Output: {"key":"value","level":"emergency","msg":"Emergency level log message"}
}

func ExampleLogger_Error() {
	logger := Logger{}
	logger.Error("Error level log message", "key", "value")
	//Output: {"key":"value","level":"error","msg":"Error level log message"}
}

func ExampleLogger_Info() {
	logger := Logger{}
	logger.Info("Info level log message", "key", "value")
	//Output: {"key":"value","level":"info","msg":"Info level log message"}
}

func ExampleLogger_Notice() {
	logger := Logger{}
	logger.Notice("Notice level log message", "key", "value")
	//Output: {"key":"value","level":"notice","msg":"Notice level log message"}
}

func ExampleLogger_Warning() {
	logger := Logger{}
	logger.Warning("Warning level log message", "key", "value")
	//Output: {"key":"value","level":"warning","msg":"Warning level log message"}
}
