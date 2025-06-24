import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.assertEquals

class MessageServiceTest {
    
    @Test
    fun `getGreeting should return Hello World message`() {
        val messageService = MessageService()
        val result = messageService.getGreeting()
        assertEquals("Hello, World!", result)
    }
}