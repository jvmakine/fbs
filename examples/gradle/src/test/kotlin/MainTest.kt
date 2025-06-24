import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.assertEquals

class MainTest {
    @Test
    fun testMain() {
        assertEquals("Hello, World!", "Hello, World!")
    }
    
    @Test
    fun testSomething() {
        assertEquals(2 + 2, 4)
    }
}