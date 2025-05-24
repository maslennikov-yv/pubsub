package pubsub

import (
	"testing"
	"time"
)

// TestREADMEExample tests the exact scenario described in README
func TestREADMEExample(t *testing.T) {
	// Создаем хаб pub-sub
	hub := NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()

	// Подписываемся на события по строковому ключу
	subscriber.Subscribe("foo")
	subscriber.Subscribe("buz")

	// Публикуем события из других горутин
	go func() {
		time.Sleep(10 * time.Millisecond)

		success1 := hub.Publish("foo", map[string]int{"foo": 90})
		if !success1 {
			t.Errorf("Expected first publish to 'foo' to succeed")
		}

		success2 := hub.Publish("foo", map[string]int{"foo": 100}) // перезаписывает предыдущее
		if !success2 {
			t.Errorf("Expected second publish to 'foo' to succeed")
		}

		success3 := hub.Publish("bar", map[string]int{"bar": 50}) // нет подписчиков
		if success3 {
			t.Errorf("Expected publish to 'bar' to fail (no subscribers)")
		}
	}()

	// Ждем до 1 секунды для событий
	results := subscriber.Wait(time.Second * 1)

	// Проверяем результаты
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d. Results: %v", len(results), results)
	}

	// Проверяем, что есть только "foo" с последним значением
	fooResult, exists := results["foo"]
	if !exists {
		t.Fatal("Expected 'foo' key in results")
	}

	fooMap, ok := fooResult.(map[string]int)
	if !ok {
		t.Fatalf("Expected 'foo' result to be map[string]int, got %T", fooResult)
	}

	if fooMap["foo"] != 100 {
		t.Fatalf("Expected 'foo' value to be 100 (last published), got %d", fooMap["foo"])
	}

	// Проверяем, что "buz" отсутствует (ничего не было опубликовано)
	if _, exists := results["buz"]; exists {
		t.Fatal("Expected 'buz' to be absent from results (nothing published)")
	}

	// Проверяем, что "bar" отсутствует (нет подписчиков)
	if _, exists := results["bar"]; exists {
		t.Fatal("Expected 'bar' to be absent from results (no subscribers)")
	}
}

// TestREADMEOverwriteBehavior tests the overwrite behavior specifically mentioned in README
func TestREADMEOverwriteBehavior(t *testing.T) {
	hub := NewPubSub()
	defer hub.Close()

	subscriber := hub.NewSubscriber()
	subscriber.Subscribe("sensor")

	// Use a channel to synchronize the start of publishing
	startPublish := make(chan struct{})

	go func() {
		<-startPublish // Wait for signal to start publishing
		// Публикуем несколько значений для одного ключа быстро подряд
		// до того, как subscriber завершит Wait()
		hub.Publish("sensor", map[string]interface{}{"value": 10, "timestamp": 1})
		hub.Publish("sensor", map[string]interface{}{"value": 20, "timestamp": 2})
		hub.Publish("sensor", map[string]interface{}{"value": 30, "timestamp": 3})
	}()

	// Start publishing and then immediately wait
	close(startPublish)
	results := subscriber.Wait(100 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	sensorData := results["sensor"].(map[string]interface{})

	// Должно быть одно из опубликованных значений (поведение перезаписи)
	// Поскольку события публикуются быстро, любое из них может быть финальным
	value := sensorData["value"]
	timestamp := sensorData["timestamp"]

	// Проверяем, что получили валидное значение
	if value != 10 && value != 20 && value != 30 {
		t.Fatalf("Expected one of published values (10, 20, 30), got %v", value)
	}

	// Проверяем соответствие timestamp и value
	expectedPairs := map[interface{}]interface{}{
		10: 1,
		20: 2,
		30: 3,
	}

	if expectedPairs[value] != timestamp {
		t.Fatalf("Timestamp %v doesn't match value %v", timestamp, value)
	}

	t.Logf("Received value: %v, timestamp: %v (overwrite behavior working correctly)", value, timestamp)
}

// TestREADMEPublishReturnValues tests the return values of Publish method as described in README
func TestREADMEPublishReturnValues(t *testing.T) {
	hub := NewPubSub()
	defer hub.Close()

	// Тест 1: Публикация без подписчиков должна возвращать false
	success := hub.Publish("nonexistent", "data")
	if success {
		t.Fatal("Expected Publish to return false when no subscribers")
	}

	// Тест 2: Публикация с подписчиками должна возвращать true
	subscriber := hub.NewSubscriber()
	subscriber.Subscribe("existing")

	success = hub.Publish("existing", "data")
	if !success {
		t.Fatal("Expected Publish to return true when subscribers exist")
	}

	// Тест 3: После завершения подписчика публикация должна возвращать false
	subscriber.Wait(1 * time.Millisecond) // Завершаем подписчика

	// Даем время на очистку
	time.Sleep(10 * time.Millisecond)

	success = hub.Publish("existing", "data")
	if success {
		t.Fatal("Expected Publish to return false after subscriber completed")
	}
}

// TestREADMETimeoutBehavior tests timeout vs early completion as described in README
func TestREADMETimeoutBehavior(t *testing.T) {
	hub := NewPubSub()
	defer hub.Close()

	// Тест 1: Раннее завершение при получении всех событий
	subscriber1 := hub.NewSubscriber()
	subscriber1.Subscribe("quick")

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.Publish("quick", "data")
	}()

	start := time.Now()
	results1 := subscriber1.Wait(1 * time.Second) // Длинный таймаут
	elapsed1 := time.Since(start)

	if len(results1) != 1 {
		t.Fatalf("Expected 1 result for early completion, got %d", len(results1))
	}

	if elapsed1 > 100*time.Millisecond {
		t.Fatalf("Expected early completion, took %v", elapsed1)
	}

	// Тест 2: Таймаут при неполучении всех событий
	subscriber2 := hub.NewSubscriber()
	subscriber2.Subscribe("slow1")
	subscriber2.Subscribe("slow2") // Это событие никогда не придет

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.Publish("slow1", "data")
		// slow2 не публикуем
	}()

	start = time.Now()
	results2 := subscriber2.Wait(50 * time.Millisecond) // Короткий таймаут
	elapsed2 := time.Since(start)

	if len(results2) != 1 {
		t.Fatalf("Expected 1 result for timeout case, got %d", len(results2))
	}

	if elapsed2 < 45*time.Millisecond || elapsed2 > 100*time.Millisecond {
		t.Fatalf("Expected timeout around 50ms, got %v", elapsed2)
	}

	// Должно быть только событие slow1
	if results2["slow1"] != "data" {
		t.Fatalf("Expected 'slow1' data, got %v", results2["slow1"])
	}

	if _, exists := results2["slow2"]; exists {
		t.Fatal("Expected 'slow2' to be absent (not published)")
	}
}
